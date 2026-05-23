const defaultSettings = {
  mode: "balanced",
  language: "zh-CN",
  insertion: "cursor",
  showInterim: true,
  enableCommands: true,
  hotwords: [
    { source: "欧喷 AI", target: "OpenAI" },
    { source: "大模型", target: "大模型" },
    { source: "flow voice", target: "FlowVoice" },
  ],
};

const simulatedSegments = [
  { interim: "今天我们先确认语音输入法的核心目标", final: "今天我们先确认语音输入法的核心目标。" },
  { interim: "第一是准确识别专业词汇", final: "第一是准确识别专业词汇。" },
  { interim: "第二是让用户一键开始 句号", final: "第二是让用户一键开始 句号" },
  { interim: "换行", final: "换行" },
  { interim: "最后补充成本要足够低", final: "最后补充成本要足够低。" },
];

class WebSpeechProvider {
  constructor(onEvent) {
    this.onEvent = onEvent;
    this.recognition = null;
    this.running = false;
  }

  isSupported() {
    return Boolean(window.SpeechRecognition || window.webkitSpeechRecognition);
  }

  start(config) {
    if (!this.isSupported()) {
      throw new Error("当前浏览器不支持 Web Speech API。");
    }

    if (this.recognition) {
      this.stop();
    }

    const Recognition = window.SpeechRecognition || window.webkitSpeechRecognition;
    const recognition = new Recognition();
    recognition.lang = config.language;
    recognition.continuous = true;
    recognition.interimResults = config.showInterim;
    recognition.maxAlternatives = 1;

    recognition.onstart = () => {
      this.running = true;
      this.onEvent({ type: "start" });
    };

    recognition.onerror = (event) => {
      this.running = false;
      this.onEvent({ type: "error", message: event.error || "识别失败" });
    };

    recognition.onend = () => {
      const shouldRestart = this.running;
      this.onEvent({ type: "end" });
      if (shouldRestart) {
        recognition.start();
      }
    };

    recognition.onresult = (event) => {
      let interim = "";
      const finals = [];

      for (let index = event.resultIndex; index < event.results.length; index += 1) {
        const result = event.results[index];
        const transcript = result[0].transcript;
        if (result.isFinal) {
          finals.push(transcript);
        } else {
          interim += transcript;
        }
      }

      if (interim) {
        this.onEvent({ type: "interim", text: interim });
      }

      finals.forEach((text) => {
        this.onEvent({ type: "final", text });
      });
    };

    this.recognition = recognition;
    this.running = true;
    recognition.start();
  }

  stop() {
    if (!this.recognition) {
      this.running = false;
      return;
    }

    const recognition = this.recognition;
    this.running = false;
    this.recognition = null;
    recognition.onend = null;
    recognition.stop();
  }
}

class MockProvider {
  constructor(onEvent) {
    this.onEvent = onEvent;
    this.timer = null;
  }

  stop() {
    if (this.timer) {
      clearTimeout(this.timer);
      this.timer = null;
    }
    this.onEvent({ type: "end" });
  }

  async simulate(segments) {
    this.onEvent({ type: "start" });

    for (const segment of segments) {
      this.onEvent({ type: "interim", text: segment.interim });
      await delay(500);
      this.onEvent({ type: "final", text: segment.final });
      await delay(180);
    }

    this.onEvent({ type: "interim", text: "" });
    this.onEvent({ type: "end" });
  }
}

const elements = {
  toggleButton: document.querySelector("#toggleButton"),
  simulateButton: document.querySelector("#simulateButton"),
  undoButton: document.querySelector("#undoButton"),
  clearButton: document.querySelector("#clearButton"),
  sendButton: document.querySelector("#sendButton"),
  copyButton: document.querySelector("#copyButton"),
  downloadButton: document.querySelector("#downloadButton"),
  addHotwordButton: document.querySelector("#addHotwordButton"),
  modeSelect: document.querySelector("#modeSelect"),
  languageSelect: document.querySelector("#languageSelect"),
  insertionSelect: document.querySelector("#insertionSelect"),
  interimToggle: document.querySelector("#interimToggle"),
  commandToggle: document.querySelector("#commandToggle"),
  statusBadge: document.querySelector("#statusBadge"),
  statusText: document.querySelector("#statusText"),
  editor: document.querySelector("#editor"),
  interimBox: document.querySelector("#interimBox"),
  hotwordList: document.querySelector("#hotwordList"),
  reviewQueue: document.querySelector("#reviewQueue"),
  queueHint: document.querySelector("#queueHint"),
  desktopBadge: document.querySelector("#desktopBadge"),
  desktopStatusText: document.querySelector("#desktopStatusText"),
  desktopOpenButton: document.querySelector("#desktopOpenButton"),
  charCount: document.querySelector("#charCount"),
  sessionTime: document.querySelector("#sessionTime"),
  inputSpeed: document.querySelector("#inputSpeed"),
  hotwordTemplate: document.querySelector("#hotwordTemplate"),
  queueItemTemplate: document.querySelector("#queueItemTemplate"),
};

const state = {
  settings: cloneSettings(defaultSettings),
  activeProvider: null,
  mockProvider: null,
  isRecording: false,
  lastInsertedSegments: [],
  reviewQueue: [],
  sessionStartAt: null,
  timerId: null,
  backendReady: false,
  desktopStatus: null,
  desktopPollId: null,
};

void bootstrap();

async function bootstrap() {
  bindEvents();

  state.activeProvider = new WebSpeechProvider(handleProviderEvent);
  state.mockProvider = new MockProvider(handleProviderEvent);

  await loadSettingsFromServer();
  hydrateControls();
  renderHotwords();
  renderReviewQueue();
  updateMetrics();
  await loadDesktopStatus();
  startDesktopPolling();

  if (!state.activeProvider.isSupported()) {
    setStatus("error", "当前浏览器不支持原生语音识别，可以先用模拟输入体验流程。");
  } else if (state.backendReady) {
    setStatus("idle", "Go 后端已连接，可以开始语音输入。");
  }
}

function bindEvents() {
  elements.toggleButton.addEventListener("click", toggleRecording);
  elements.simulateButton.addEventListener("click", runSimulation);
  elements.undoButton.addEventListener("click", undoLastInsert);
  elements.clearButton.addEventListener("click", clearEditor);
  elements.sendButton.addEventListener("click", sendToDesktopTarget);
  elements.copyButton.addEventListener("click", copyEditorText);
  elements.downloadButton.addEventListener("click", downloadTextFile);
  elements.editor.addEventListener("input", updateMetrics);
  elements.desktopOpenButton.addEventListener("click", openDesktopWindow);

  elements.addHotwordButton.addEventListener("click", async () => {
    state.settings.hotwords.push({ source: "", target: "" });
    renderHotwords();
    await persistSettings();
  });

  [
    [elements.modeSelect, "mode"],
    [elements.languageSelect, "language"],
    [elements.insertionSelect, "insertion"],
  ].forEach(([control, key]) => {
    control.addEventListener("change", async () => {
      state.settings[key] = control.value;
      refreshModeHints();
      await persistSettings();
    });
  });

  [
    [elements.interimToggle, "showInterim"],
    [elements.commandToggle, "enableCommands"],
  ].forEach(([control, key]) => {
    control.addEventListener("change", async () => {
      state.settings[key] = control.checked;
      await persistSettings();
    });
  });
}

function hydrateControls() {
  elements.modeSelect.value = state.settings.mode;
  elements.languageSelect.value = state.settings.language;
  elements.insertionSelect.value = state.settings.insertion;
  elements.interimToggle.checked = state.settings.showInterim;
  elements.commandToggle.checked = state.settings.enableCommands;
  refreshModeHints();
}

function refreshModeHints() {
  if (state.settings.mode === "speed") {
    elements.queueHint.textContent = "极速模式会跳过待确认队列，直接写入文本。";
  } else if (state.settings.mode === "review") {
    elements.queueHint.textContent = "审校模式会先进入待确认队列，再由用户确认。";
  } else {
    elements.queueHint.textContent = "平衡模式默认直接写入，并保留轻量纠错。";
  }
}

async function loadSettingsFromServer() {
  try {
    const response = await fetch("/api/settings");
    if (!response.ok) {
      throw new Error(`load settings failed: ${response.status}`);
    }

    state.settings = await response.json();
    state.backendReady = true;
  } catch (error) {
    console.error(error);
    state.settings = cloneSettings(defaultSettings);
    state.backendReady = false;
    setStatus("error", "Go 后端暂时不可用，已回退到默认设置。");
  }
}

async function persistSettings() {
  try {
    const response = await fetch("/api/settings", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(state.settings),
    });

    if (!response.ok) {
      throw new Error(`save settings failed: ${response.status}`);
    }

    state.settings = await response.json();
    state.backendReady = true;
  } catch (error) {
    console.error(error);
    state.backendReady = false;
    setStatus("error", "设置保存失败，Go 后端未响应。");
  }
}

async function loadDesktopStatus() {
  try {
    const response = await fetch("/api/desktop/status");
    if (!response.ok) {
      throw new Error(`load desktop status failed: ${response.status}`);
    }

    state.desktopStatus = await response.json();
  } catch (error) {
    console.error(error);
    state.desktopStatus = {
      enabled: false,
      platform: navigator.platform,
      mode: "web",
      note: "桌面状态读取失败。",
      lastError: "桌面状态读取失败",
    };
  }

  renderDesktopStatus();
}

function startDesktopPolling() {
  stopDesktopPolling();
  state.desktopPollId = window.setInterval(() => {
    void loadDesktopStatus();
  }, 3000);
}

function stopDesktopPolling() {
  if (state.desktopPollId) {
    window.clearInterval(state.desktopPollId);
    state.desktopPollId = null;
  }
}

function toggleRecording() {
  if (state.isRecording) {
    stopRecording();
    return;
  }

  try {
    state.activeProvider.start({
      language: state.settings.language,
      showInterim: state.settings.showInterim,
    });
    state.isRecording = true;
    state.sessionStartAt = Date.now();
    ensureTimer();
    elements.toggleButton.textContent = "停止语音输入";
    setStatus("recording", "正在监听麦克风并持续识别。");
  } catch (error) {
    setStatus("error", error.message);
  }
}

function stopRecording() {
  if (!state.isRecording) {
    return;
  }

  state.isRecording = false;
  state.activeProvider.stop();
  elements.toggleButton.textContent = "开始语音输入";
  elements.interimBox.textContent = "等待识别结果...";
  clearTimer();
  setStatus("idle", "语音输入已停止。");
}

function handleProviderEvent(event) {
  if (event.type === "interim") {
    elements.interimBox.textContent = event.text || "等待识别结果...";
    return;
  }

  if (event.type === "final") {
    void processFinalTranscript(event.text);
    elements.interimBox.textContent = "等待识别结果...";
    return;
  }

  if (event.type === "error") {
    state.isRecording = false;
    elements.toggleButton.textContent = "开始语音输入";
    clearTimer();
    setStatus("error", resolveErrorMessage(event.message));
    return;
  }

  if (event.type === "end" && !state.isRecording) {
    setStatus("idle", "语音输入已停止。");
  }
}

async function processFinalTranscript(rawText) {
  const payload = await postTranscript(rawText);
  if (!payload) {
    return;
  }

  if (payload.kind === "noop") {
    return;
  }

  if (payload.kind === "command" && payload.command) {
    applyCommand(payload.command);
    return;
  }

  if (payload.kind === "queue") {
    state.reviewQueue.unshift(payload.text);
    renderReviewQueue();
    setStatus(state.isRecording ? "recording" : "idle", payload.message || "片段已加入待确认队列。");
    return;
  }

  if (payload.kind === "insert") {
    insertText(payload.text);
    setStatus(state.isRecording ? "recording" : "idle", payload.message || "片段已写入编辑区。");
  }
}

async function postTranscript(transcript) {
  try {
    const response = await fetch("/api/process", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ transcript }),
    });

    if (!response.ok) {
      throw new Error(`process transcript failed: ${response.status}`);
    }

    state.backendReady = true;
    return await response.json();
  } catch (error) {
    console.error(error);
    state.backendReady = false;
    setStatus("error", "文本后处理失败，Go 后端未响应。");
    return null;
  }
}

function applyCommand(command) {
  if (command.type === "insert") {
    insertText(command.value, { trackUndo: false });
    setStatus(state.isRecording ? "recording" : "idle", `已执行语音命令：${command.label}`);
    return;
  }

  if (command.type === "undo") {
    undoLastInsert();
  }
}

function insertText(text, options = {}) {
  const { trackUndo = true } = options;
  const editor = elements.editor;
  const before = editor.value;

  let nextValue = before;
  let selectionStart = editor.selectionStart;
  let selectionEnd = editor.selectionEnd;

  if (state.settings.insertion === "append") {
    const prefix = before && !before.endsWith("\n") ? "\n" : "";
    nextValue = before + prefix + text;
    selectionStart = nextValue.length;
    selectionEnd = nextValue.length;
  } else {
    nextValue = `${before.slice(0, selectionStart)}${text}${before.slice(selectionEnd)}`;
    selectionStart += text.length;
    selectionEnd = selectionStart;
  }

  editor.value = nextValue;
  editor.focus();
  editor.setSelectionRange(selectionStart, selectionEnd);

  if (trackUndo) {
    state.lastInsertedSegments.push(text);
    if (state.lastInsertedSegments.length > 40) {
      state.lastInsertedSegments.shift();
    }
  }

  updateMetrics();
}

function undoLastInsert() {
  const lastSegment = state.lastInsertedSegments.pop();
  if (!lastSegment) {
    setStatus(state.isRecording ? "recording" : "idle", "没有可撤销的语音片段。");
    return;
  }

  const editor = elements.editor;
  const index = editor.value.lastIndexOf(lastSegment);
  if (index === -1) {
    setStatus(state.isRecording ? "recording" : "idle", "最近片段未找到，未执行撤销。");
    return;
  }

  editor.value = `${editor.value.slice(0, index)}${editor.value.slice(index + lastSegment.length)}`;
  editor.focus();
  editor.setSelectionRange(index, index);
  updateMetrics();
  setStatus(state.isRecording ? "recording" : "idle", "已撤销上一段语音输入。");
}

function clearEditor() {
  elements.editor.value = "";
  state.lastInsertedSegments = [];
  updateMetrics();
  setStatus(state.isRecording ? "recording" : "idle", "文本内容已清空。");
}

async function runSimulation() {
  if (!state.sessionStartAt) {
    state.sessionStartAt = Date.now();
  }

  ensureTimer();
  setStatus("recording", "正在模拟一段会议纪要输入流程。");
  await state.mockProvider.simulate(simulatedSegments);
}

function renderDesktopStatus() {
  const status = state.desktopStatus;
  if (!status) {
    return;
  }

  const badge = elements.desktopBadge;
  badge.className = `status-badge ${status.enabled ? (status.targetReady ? "recording" : "idle") : "error"}`;
  badge.textContent = status.enabled ? (status.targetReady ? "已锁定目标" : "桌面模式") : "未连接";

  let text = status.note || "桌面模式未启用。";
  if (status.lastError) {
    text += ` 当前异常：${status.lastError}`;
  } else if (status.hotkey) {
    text += ` 热键：${status.hotkey}。`;
  }

  elements.desktopStatusText.textContent = text;
  elements.sendButton.disabled = !status.enabled || !status.targetReady;
}

async function openDesktopWindow() {
  try {
    const response = await fetch("/api/desktop/open", { method: "POST" });
    if (!response.ok) {
      const payload = await response.json().catch(() => ({ error: "桌面窗口打开失败" }));
      throw new Error(payload.error || "桌面窗口打开失败");
    }

    await loadDesktopStatus();
    setStatus(state.isRecording ? "recording" : "idle", "桌面输入窗已打开。");
  } catch (error) {
    setStatus("error", error.message);
  }
}

async function sendToDesktopTarget() {
  const text = elements.editor.value.trim();
  if (!text) {
    setStatus("error", "当前没有可发送的文本。");
    return;
  }

  try {
    const response = await fetch("/api/desktop/paste", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text: elements.editor.value }),
    });

    if (!response.ok) {
      const payload = await response.json().catch(() => ({ error: "发送失败" }));
      throw new Error(payload.error || "发送失败");
    }

    await loadDesktopStatus();
    setStatus(state.isRecording ? "recording" : "idle", "文本已发送到原应用。");
  } catch (error) {
    await loadDesktopStatus();
    setStatus("error", error.message);
  }
}

function renderHotwords() {
  elements.hotwordList.innerHTML = "";

  state.settings.hotwords.forEach((item, index) => {
    const fragment = elements.hotwordTemplate.content.cloneNode(true);
    const root = fragment.querySelector(".hotword-item");
    const sourceInput = fragment.querySelector(".hotword-source");
    const targetInput = fragment.querySelector(".hotword-target");
    const deleteButton = fragment.querySelector(".icon-button");

    sourceInput.value = item.source;
    targetInput.value = item.target;

    sourceInput.addEventListener("input", async () => {
      state.settings.hotwords[index].source = sourceInput.value;
      await persistSettings();
    });

    targetInput.addEventListener("input", async () => {
      state.settings.hotwords[index].target = targetInput.value;
      await persistSettings();
    });

    deleteButton.addEventListener("click", async () => {
      state.settings.hotwords.splice(index, 1);
      renderHotwords();
      await persistSettings();
    });

    elements.hotwordList.appendChild(root);
  });
}

function renderReviewQueue() {
  const queue = elements.reviewQueue;
  queue.innerHTML = "";

  if (!state.reviewQueue.length) {
    queue.classList.add("empty");
    queue.innerHTML = "<p>当前没有待确认片段。</p>";
    return;
  }

  queue.classList.remove("empty");
  state.reviewQueue.forEach((text, index) => {
    const fragment = elements.queueItemTemplate.content.cloneNode(true);
    const textNode = fragment.querySelector(".queue-text");
    const approveButton = fragment.querySelector(".approve-button");
    const rejectButton = fragment.querySelector(".reject-button");

    textNode.textContent = text;

    approveButton.addEventListener("click", () => {
      insertText(text);
      state.reviewQueue.splice(index, 1);
      renderReviewQueue();
    });

    rejectButton.addEventListener("click", () => {
      state.reviewQueue.splice(index, 1);
      renderReviewQueue();
    });

    queue.appendChild(fragment);
  });
}

function updateMetrics() {
  const value = elements.editor.value;
  const charCount = value.replace(/\s/g, "").length;
  elements.charCount.textContent = String(charCount);

  if (!state.sessionStartAt) {
    elements.sessionTime.textContent = "00:00";
    elements.inputSpeed.textContent = "0 字/分";
    return;
  }

  const elapsedMs = Date.now() - state.sessionStartAt;
  const elapsedMinutes = Math.max(elapsedMs / 60000, 1 / 60);
  const speed = Math.round(charCount / elapsedMinutes);

  elements.sessionTime.textContent = formatDuration(elapsedMs);
  elements.inputSpeed.textContent = `${speed} 字/分`;
}

function ensureTimer() {
  clearTimer();
  state.timerId = window.setInterval(updateMetrics, 1000);
}

function clearTimer() {
  if (state.timerId) {
    window.clearInterval(state.timerId);
    state.timerId = null;
  }
}

function setStatus(kind, message) {
  elements.statusBadge.className = `status-badge ${kind}`;
  elements.statusBadge.textContent =
    kind === "recording" ? "输入中" : kind === "error" ? "异常" : "待机";
  elements.statusText.textContent = message;
}

async function copyEditorText() {
  try {
    await navigator.clipboard.writeText(elements.editor.value);
    setStatus(state.isRecording ? "recording" : "idle", "文本已复制到剪贴板。");
  } catch (error) {
    setStatus("error", "复制失败，请检查浏览器权限。");
  }
}

function downloadTextFile() {
  const blob = new Blob([elements.editor.value], { type: "text/plain;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `flowvoice-${Date.now()}.txt`;
  link.click();
  URL.revokeObjectURL(url);
}

function resolveErrorMessage(code) {
  const errorMap = {
    "audio-capture": "没有检测到可用麦克风。",
    "not-allowed": "浏览器未获得麦克风权限。",
    network: "语音识别网络异常，建议稍后重试或切换模拟模式。",
    "no-speech": "未检测到语音，请靠近麦克风后重试。",
  };

  return errorMap[code] || `语音识别失败：${code}`;
}

function formatDuration(ms) {
  const totalSeconds = Math.floor(ms / 1000);
  const minutes = String(Math.floor(totalSeconds / 60)).padStart(2, "0");
  const seconds = String(totalSeconds % 60).padStart(2, "0");
  return `${minutes}:${seconds}`;
}

function cloneSettings(settings) {
  return {
    ...settings,
    hotwords: settings.hotwords.map((item) => ({ ...item })),
  };
}

function delay(ms) {
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms);
  });
}
