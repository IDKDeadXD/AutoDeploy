package model

import "time"

const ConfigVersion = 1

type GlobalConfig struct {
	Listen        string `json:"listen"`
	Port          int    `json:"port"`
	PublicURL     string `json:"publicURL"`
	MaxConcurrent int    `json:"maxConcurrent"`
	ServiceUser   string `json:"serviceUser"`
	BinaryPath    string `json:"binaryPath"`
}

type Project struct {
	Version     int         `json:"version"`
	Name        string      `json:"name"`
	Root        string      `json:"root"`
	Token       string      `json:"token"`
	Repository  Repository  `json:"repository"`
	Deployment  Deployment  `json:"deployment"`
	HealthCheck HealthCheck `json:"healthCheck"`
	Discord     Discord     `json:"discord"`
}

type Repository struct {
	Remote         string `json:"remote"`
	Branch         string `json:"branch"`
	UpdateStrategy string `json:"updateStrategy"`
	URL            string `json:"url"`
}
type Deployment struct {
	WorkingDirectory string    `json:"workingDirectory"`
	Commands         []Command `json:"commands"`
	StopOnFailure    bool      `json:"stopOnFailure"`
}
type Command struct {
	Name           string   `json:"name"`
	Program        string   `json:"program,omitempty"`
	Args           []string `json:"args,omitempty"`
	Command        string   `json:"command,omitempty"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
}
type HealthCheck struct {
	Enabled         bool   `json:"enabled"`
	URL             string `json:"url"`
	ExpectedStatus  int    `json:"expectedStatus"`
	TimeoutSeconds  int    `json:"timeoutSeconds"`
	IntervalSeconds int    `json:"intervalSeconds"`
}
type Discord struct {
	Enabled bool   `json:"enabled"`
	Events  Events `json:"events"`
}
type Events struct {
	Started    bool `json:"started"`
	Succeeded  bool `json:"succeeded"`
	Failed     bool `json:"failed"`
	RolledBack bool `json:"rolledBack"`
}
type State struct {
	LastDetectedCommit   string `json:"lastDetectedCommit"`
	LastAttemptedCommit  string `json:"lastAttemptedCommit"`
	LastSuccessfulCommit string `json:"lastSuccessfulCommit"`
	LastFailedCommit     string `json:"lastFailedCommit"`
	Status               string `json:"status"`
	Pending              *Job   `json:"pending,omitempty"`
}
type Job struct {
	Commit     string    `json:"commit"`
	DeliveryID string    `json:"deliveryID"`
	Author     string    `json:"author"`
	Message    string    `json:"message"`
	ReceivedAt time.Time `json:"receivedAt"`
}
type Record struct {
	ID             string    `json:"id"`
	Project        string    `json:"project"`
	Status         string    `json:"status"`
	Commit         string    `json:"commit"`
	Branch         string    `json:"branch"`
	Author         string    `json:"author"`
	Message        string    `json:"message"`
	FailedStep     string    `json:"failedStep,omitempty"`
	Error          string    `json:"error,omitempty"`
	ExitCode       int       `json:"exitCode,omitempty"`
	StartedAt      time.Time `json:"startedAt"`
	EndedAt        time.Time `json:"endedAt"`
	DurationMillis int64     `json:"durationMillis"`
}
