# FlowVoice 语音输入法

FlowVoice 现在已经从单纯的网页原型，推进到一个可运行的 Windows 桌面级语音输入法 MVP。

## 当前形态

这版不是完整的 Windows TSF 系统输入法，但已经实现了接近真实使用的核心链路：

1. 在任意应用里按全局热键 `Ctrl+Alt+V`
2. FlowVoice 以桌面输入窗形式被唤起
3. 用户进行语音输入、编辑、审校
4. 点击“发送到当前应用”
5. 后端自动切回原应用并执行粘贴

这意味着它已经具备“跨应用输入”的核心价值，只是底层机制仍然是“桌面助手 + 自动回填”，而不是 TSF/IMM 级别的原生输入法。

## 为什么先做成这样

如果直接做系统级输入法，要进入 Windows TSF/IMM 路线，复杂度和风险都会显著上升：

- 需要深度处理输入上下文、候选框、组合串、焦点切换
- Windows 原生 IME 更适合 C++ / COM / TSF 生态
- 调试成本、兼容性成本、交付周期都明显更高

对你当前的产品目标来说，先做“桌面级 MVP”更合理，因为它已经能验证最重要的事情：

- 用户是否愿意用语音替代键盘录入
- 热词、命令、审校机制是否真的降低纠错成本
- 自动回填到任意应用的体验是否足够顺滑

## 架构

### 前端

- 浏览器端语音采集
- 实时草稿显示
- 文本编辑区
- 审校队列
- 热词维护
- 发送到当前应用

### Go 后端

- 静态资源服务
- 设置持久化
- 识别结果后处理
- 热词替换
- 语音命令解析
- 桌面模式状态接口
- Windows 桌面集成
  - 全局热键
  - 唤起 Edge App 模式输入窗
  - 切回目标应用
  - 自动粘贴文本

## 主要接口

- `GET /api/health`
- `GET /api/settings`
- `PUT /api/settings`
- `POST /api/process`
- `GET /api/desktop/status`
- `POST /api/desktop/open`
- `POST /api/desktop/paste`

## 运行方式

### Web 模式

适合继续调试前端或后端规则：

```powershell
cd D:\FlowVoice
go run main.go
```

或：

```powershell
powershell -ExecutionPolicy Bypass -File .\serve.ps1
```

### Desktop 模式

适合真实跨应用输入体验：

```powershell
cd D:\FlowVoice
go run main.go -mode desktop -open
```

或：

```powershell
powershell -ExecutionPolicy Bypass -File .\desktop.ps1
```

启动后建议这样使用：

1. 先切到目标应用，比如记事本、聊天框、文档编辑器
2. 按 `Ctrl+Alt+V`
3. 在 FlowVoice 窗口中完成语音输入
4. 点击“发送到当前应用”

## 当前限制

1. 输入窗目前通过 Edge App 模式承载，还不是独立原生桌面壳
2. 文本回填目前走剪贴板 + 粘贴，不是原生输入法上下文注入
3. 全局热键和焦点恢复目前只实现了 Windows
4. 还没有托盘、开机自启、权限检查、日志诊断这些桌面产品细节

## 当前文件

- [main.go](D:/FlowVoice/main.go)
- [desktop_windows.go](D:/FlowVoice/desktop_windows.go)
- [desktop_stub.go](D:/FlowVoice/desktop_stub.go)
- [app.js](D:/FlowVoice/app.js)
- [index.html](D:/FlowVoice/index.html)
- [styles.css](D:/FlowVoice/styles.css)
- [desktop.ps1](D:/FlowVoice/desktop.ps1)
- [serve.ps1](D:/FlowVoice/serve.ps1)
