package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/idkde/deploy-agent/internal/model"
)

func signature(secret string, body []byte) string {
	h := hmac.New(sha256.New, []byte(secret))
	_, _ = h.Write(body)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}
func project() model.Project {
	return model.Project{Repository: model.Repository{Branch: "main", URL: "https://github.com/acme/widget.git"}}
}
func TestVerify(t *testing.T) {
	body := []byte("payload")
	if !Verify("secret", body, signature("secret", body)) {
		t.Fatal("valid signature rejected")
	}
	if Verify("secret", body, "sha256=00") {
		t.Fatal("invalid signature accepted")
	}
	if Verify("secret", body, "") {
		t.Fatal("missing signature accepted")
	}
}
func TestParsePushFiltersBranchAndRepository(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main","after":"0123456789012345678901234567890123456789","repository":{"clone_url":"https://github.com/acme/widget.git"},"head_commit":{"message":"ship","author":{"name":"A"}}}`)
	j, err := ParsePush(body, project())
	if err != nil || j.Commit == "" {
		t.Fatalf("valid push rejected: %v", err)
	}
	wrong := project()
	wrong.Repository.Branch = "production"
	if _, err := ParsePush(body, wrong); err == nil {
		t.Fatal("wrong branch accepted")
	}
	wrong = project()
	wrong.Repository.URL = "https://github.com/acme/other.git"
	if _, err := ParsePush(body, wrong); err == nil {
		t.Fatal("wrong repository accepted")
	}
}
