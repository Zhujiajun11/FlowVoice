//go:build windows

package desktop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	wmHotkey       = 0x0312
	modAlt         = 0x0001
	modControl     = 0x0002
	modNoRepeat    = 0x4000
	vkV            = 0x56
	swRestore      = 9
	cfUnicodeText  = 13
	gmemMoveable   = 0x0002
	keyeventfKeyup = 0x0002
)

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procRegisterHotKey      = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey    = user32.NewProc("UnregisterHotKey")
	procGetMessage          = user32.NewProc("GetMessageW")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procShowWindow          = user32.NewProc("ShowWindow")
	procIsIconic            = user32.NewProc("IsIconic")
	procGetWindowThreadPID  = user32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput   = user32.NewProc("AttachThreadInput")
	procBringWindowToTop    = user32.NewProc("BringWindowToTop")
	procOpenClipboard       = user32.NewProc("OpenClipboard")
	procCloseClipboard      = user32.NewProc("CloseClipboard")
	procEmptyClipboard      = user32.NewProc("EmptyClipboard")
	procSetClipboardData    = user32.NewProc("SetClipboardData")
	procKeybdEvent          = user32.NewProc("keybd_event")
	procGetCurrentThreadID  = kernel32.NewProc("GetCurrentThreadId")
	procGlobalAlloc         = kernel32.NewProc("GlobalAlloc")
	procGlobalLock          = kernel32.NewProc("GlobalLock")
	procGlobalUnlock        = kernel32.NewProc("GlobalUnlock")
)

type winPoint struct {
	X int32
	Y int32
}

type winMsg struct {
	HWnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       winPoint
	LPrivate uint32
}

type windowsAgent struct {
	mu           sync.RWMutex
	uiURL        string
	edgePath     string
	hotkey       string
	targetWindow uintptr
	lastError    string
}

func New(uiURL string) (Integration, error) {
	edgePath, err := findEdgePath()
	if err != nil {
		return nil, err
	}

	agent := &windowsAgent{
		uiURL:    uiURL,
		edgePath: edgePath,
		hotkey:   "Ctrl+Alt+V",
	}

	go agent.runHotkeyLoop()
	return agent, nil
}

func (a *windowsAgent) Status() Status {
	a.mu.RLock()
	defer a.mu.RUnlock()

	status := Status{
		Enabled:     true,
		Platform:    runtime.GOOS,
		Mode:        "desktop",
		Hotkey:      a.hotkey,
		TargetReady: a.targetWindow != 0,
		WindowMode:  "msedge-app",
		LastError:   a.lastError,
		Note:        "按 Ctrl+Alt+V 可从任意应用唤起输入窗，完成后点击“发送到当前应用”。",
	}

	if status.TargetReady {
		status.Note = "目标窗口已锁定，发送文本时会自动切回原应用并粘贴。"
	}

	return status
}

func (a *windowsAgent) OpenUI() error {
	return a.openUI(false)
}

func (a *windowsAgent) PasteText(text string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("没有可发送的文本")
	}

	target := a.getTargetWindow()
	if target == 0 {
		return errors.New("没有锁定目标窗口，请先在目标应用中按 Ctrl+Alt+V 唤起输入法")
	}

	if err := writeClipboardText(text); err != nil {
		a.setLastError(err)
		return fmt.Errorf("写入剪贴板失败: %w", err)
	}

	if err := focusWindow(target); err != nil {
		a.setLastError(err)
		return fmt.Errorf("切回目标窗口失败: %w", err)
	}

	time.Sleep(150 * time.Millisecond)
	sendCtrlV()
	a.clearLastError()
	return nil
}

func (a *windowsAgent) runHotkeyLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	const hotkeyID = 1
	result, _, err := procRegisterHotKey.Call(
		0,
		uintptr(hotkeyID),
		uintptr(modControl|modAlt|modNoRepeat),
		uintptr(vkV),
	)
	if result == 0 {
		a.setLastError(fmt.Errorf("注册全局热键失败: %v", err))
		return
	}
	defer procUnregisterHotKey.Call(0, uintptr(hotkeyID))

	var msg winMsg
	for {
		ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			return
		}

		if msg.Message == wmHotkey && msg.WParam == uintptr(hotkeyID) {
			if hwnd, hwndErr := currentForegroundWindow(); hwndErr == nil && hwnd != 0 {
				a.setTargetWindow(hwnd)
			}
			if openErr := a.openUI(true); openErr != nil {
				a.setLastError(openErr)
			}
		}
	}
}

func (a *windowsAgent) openUI(fromHotkey bool) error {
	args := []string{
		"--app=" + a.uiURL,
		"--window-size=1180,920",
	}

	profileDir := filepath.Join(os.TempDir(), "flowvoice-edge-profile")
	args = append(args, "--user-data-dir="+profileDir)

	cmd := exec.Command(a.edgePath, args...)
	if err := cmd.Start(); err != nil {
		return err
	}

	if fromHotkey {
		a.clearLastError()
	}
	return nil
}

func (a *windowsAgent) getTargetWindow() uintptr {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.targetWindow
}

func (a *windowsAgent) setTargetWindow(hwnd uintptr) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.targetWindow = hwnd
}

func (a *windowsAgent) setLastError(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastError = err.Error()
}

func (a *windowsAgent) clearLastError() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastError = ""
}

func findEdgePath() (string, error) {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", errors.New("未找到 Microsoft Edge，桌面输入窗无法使用")
}

func currentForegroundWindow() (uintptr, error) {
	hwnd, _, err := procGetForegroundWindow.Call()
	if hwnd == 0 {
		if err != syscall.Errno(0) {
			return 0, err
		}
		return 0, errors.New("无法获取前台窗口")
	}
	return hwnd, nil
}

func focusWindow(hwnd uintptr) error {
	current, _, _ := procGetForegroundWindow.Call()
	thisThread, _, _ := procGetCurrentThreadID.Call()
	targetThread, _, _ := procGetWindowThreadPID.Call(hwnd, 0)
	currentThread, _, _ := procGetWindowThreadPID.Call(current, 0)

	if currentThread != 0 {
		procAttachThreadInput.Call(thisThread, currentThread, 1)
		defer procAttachThreadInput.Call(thisThread, currentThread, 0)
	}
	if targetThread != 0 && targetThread != currentThread {
		procAttachThreadInput.Call(thisThread, targetThread, 1)
		defer procAttachThreadInput.Call(thisThread, targetThread, 0)
	}

	iconic, _, _ := procIsIconic.Call(hwnd)
	if iconic != 0 {
		procShowWindow.Call(hwnd, swRestore)
	}
	procBringWindowToTop.Call(hwnd)
	result, _, err := procSetForegroundWindow.Call(hwnd)
	if result == 0 {
		return err
	}
	return nil
}

func writeClipboardText(text string) error {
	if ok, _, err := procOpenClipboard.Call(0); ok == 0 {
		return err
	}
	defer procCloseClipboard.Call()

	if ok, _, err := procEmptyClipboard.Call(); ok == 0 {
		return err
	}

	encoded := utf16.Encode([]rune(text + "\x00"))
	size := uintptr(len(encoded) * 2)

	handle, _, err := procGlobalAlloc.Call(gmemMoveable, size)
	if handle == 0 {
		return err
	}

	ptr, _, err := procGlobalLock.Call(handle)
	if ptr == 0 {
		return err
	}

	buffer := unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(encoded))
	copy(buffer, encoded)
	procGlobalUnlock.Call(handle)

	if ok, _, err := procSetClipboardData.Call(cfUnicodeText, handle); ok == 0 {
		return err
	}

	return nil
}

func sendCtrlV() {
	procKeybdEvent.Call(uintptr(0x11), 0, 0, 0)
	procKeybdEvent.Call(uintptr(vkV), 0, 0, 0)
	procKeybdEvent.Call(uintptr(vkV), 0, keyeventfKeyup, 0)
	procKeybdEvent.Call(uintptr(0x11), 0, keyeventfKeyup, 0)
}
