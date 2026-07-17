package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/idkde/deploy-agent/internal/model"
)

func Discord(ctx context.Context, url string, r model.Record) error {
	title := "Deployment successful"
	color := 0x2ecc71
	if r.Status != "success" {
		title = "Deployment failed"
		color = 0xe74c3c
	}
	fields := []map[string]any{{"name": "Project", "value": r.Project, "inline": true}, {"name": "Branch", "value": r.Branch, "inline": true}, {"name": "Commit", "value": short(r.Commit), "inline": true}, {"name": "Duration", "value": time.Duration(r.DurationMillis * int64(time.Millisecond)).Round(time.Second).String(), "inline": true}}
	if r.FailedStep != "" {
		fields = append(fields, map[string]any{"name": "Failed step", "value": r.FailedStep, "inline": false})
	}
	body, _ := json.Marshal(map[string]any{"embeds": []any{map[string]any{"title": title, "description": sanitize(r.Message), "color": color, "fields": fields}}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("Discord returned %s", res.Status)
	}
	return nil
}
func short(v string) string {
	if len(v) > 7 {
		return v[:7]
	}
	return v
}
func sanitize(v string) string {
	v = strings.ReplaceAll(v, "@", "@​")
	if len(v) > 512 {
		return v[:512]
	}
	return v
}
