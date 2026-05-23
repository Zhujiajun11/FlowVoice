package desktop

type Status struct {
	Enabled     bool   `json:"enabled"`
	Platform    string `json:"platform"`
	Mode        string `json:"mode"`
	Hotkey      string `json:"hotkey,omitempty"`
	TargetReady bool   `json:"targetReady"`
	WindowMode  string `json:"windowMode,omitempty"`
	LastError   string `json:"lastError,omitempty"`
	Note        string `json:"note,omitempty"`
}

type Integration interface {
	Status() Status
	OpenUI() error
	PasteText(text string) error
}
