package entity

import "errors"

// InstallState says where the files of a model or of the runtime are.
type InstallState string

const (
	InstallNotDownloaded InstallState = "not_downloaded"
	InstallPartial       InstallState = "partial" // a download was interrupted and can continue
	InstallDownloading   InstallState = "downloading"
	InstallReady         InstallState = "ready"
)

// FileInstall is the state of one downloadable thing. DownloadID is the id of the running download (follow it with
// GET /api/v1/downloads/{id}); PartialBytes is how much of an interrupted download is already on disk.
type FileInstall struct {
	State        InstallState `json:"state"`
	PartialBytes int64        `json:"partialBytes"`
	DownloadID   string       `json:"downloadId"`
}

// Reasons an install request is refused.
var (
	ErrUnknownModel      = errors.New("the model is not one of the models Cinqo offers")
	ErrUnsupportedSystem = errors.New("this system is not supported yet")
	ErrDownloadRunning   = errors.New("a download is running")
)

// Failure says why the last download of a thing (the program that runs the models, or a model) did not finish. What is "program" or
// "model"; Message is fit to show; At is when it ended (RFC 3339).
type Failure struct {
	What    string `json:"what"`
	Message string `json:"message"`
	At      string `json:"at"`
}
