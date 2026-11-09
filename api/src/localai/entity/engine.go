package entity

// EngineState is where the model program (llama-server, started by Cinqo) is.
type EngineState string

const (
	EngineStopped  EngineState = "stopped"  // not running; it starts when a conversation needs it
	EngineStarting EngineState = "starting" // the program is loading the model
	EngineReady    EngineState = "ready"
	EngineFailed   EngineState = "failed"
)

// UnloadResult is what a request to unload the model program came to.
type UnloadResult string

const (
	UnloadStopped UnloadResult = "stopped" // the program was stopped, or was not running
	UnloadInUse   UnloadResult = "in_use"  // a conversation is using the model right now and the caller did not force it
)

// EngineStatus is what the settings page shows about the model program: its state, why it failed (when it did) and which
// model it runs or last ran.
type EngineStatus struct {
	State   EngineState `json:"state"`
	Error   string      `json:"error"`
	ModelID string      `json:"modelId"`
}

// TestResult is the outcome of the "test the model" request: one tiny real conversation, timed. StartSeconds is how long the
// model program needed to become ready (near zero when it was already running), Seconds how long the answer took, and
// TokensPerSecond the completion tokens over those seconds (the answer's overall speed, prompt included).
type TestResult struct {
	OK               bool    `json:"ok"`
	Error            string  `json:"error"`
	StartSeconds     float64 `json:"startSeconds"`
	Seconds          float64 `json:"seconds"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	TokensPerSecond  float64 `json:"tokensPerSecond"`
	Reply            string  `json:"reply"`
}
