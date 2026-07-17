package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/idkde/deploy-agent/internal/config"
	"github.com/idkde/deploy-agent/internal/model"
)

type Push struct {
	Ref, After string
	Deleted    bool
	Repository struct {
		CloneURL string `json:"clone_url"`
		SSHURL   string `json:"ssh_url"`
	}
	HeadCommit struct {
		Message string
		Author  struct{ Name string }
	} `json:"head_commit"`
}

func Verify(secret string, body []byte, signature string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(signature, prefix) {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(signature, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hmac.Equal(got, mac.Sum(nil))
}
func ParsePush(body []byte, p model.Project) (model.Job, error) {
	var event Push
	if err := json.Unmarshal(body, &event); err != nil {
		return model.Job{}, errors.New("malformed GitHub payload")
	}
	if event.Deleted {
		return model.Job{}, errors.New("deleted branch")
	}
	if event.Ref != "refs/heads/"+p.Repository.Branch {
		return model.Job{}, errors.New("branch does not match")
	}
	if !config.RemoteMatches(p.Repository.URL, event.Repository.CloneURL) && !config.RemoteMatches(p.Repository.URL, event.Repository.SSHURL) {
		return model.Job{}, errors.New("repository does not match")
	}
	if len(event.After) != 40 {
		return model.Job{}, errors.New("payload has invalid commit")
	}
	return model.Job{Commit: event.After, Author: event.HeadCommit.Author.Name, Message: event.HeadCommit.Message}, nil
}
