package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
)

type commandHelp struct {
	Usage       string
	Description string
}

var commandGroups = []struct {
	Title    string
	Commands []commandHelp
}{
	{"Setup", []commandHelp{
		{"install --user <name> [--listen ip] [--port n] [--public-url url]", "install and enable the systemd daemon"},
		{"uninstall", "remove the systemd unit without deleting project state"},
		{"update [--repo owner/name]", "update the user-owned binary from a GitHub release"},
		{"init", "register the current Git repository"},
	}},
	{"Projects", []commandHelp{
		{"list", "show registered projects"},
		{"status [--project name]", "show project state"},
		{"remove [--project name]", "unregister a project"},
	}},
	{"Deployments", []commandHelp{
		{"run [--project name] [--commit sha] [--dry-run]", "queue or preview a deployment"},
		{"history [--project name]", "show recent deployment records"},
		{"logs [--project name]", "show deployment log summaries"},
		{"doctor [--project name]", "run local health checks"},
	}},
	{"Configuration", []commandHelp{
		{"config validate [--project name]", "validate project configuration"},
		{"config command <shell-command> [--project name]", "replace the deployment command"},
		{"webhook show [--project name]", "show the GitHub webhook URL and masked secret"},
		{"webhook reveal --yes [--project name]", "print the webhook secret"},
		{"webhook rotate [--project name]", "rotate the webhook secret"},
	}},
	{"Integrations", []commandHelp{
		{"notifications discord setup [--project name]", "store a Discord webhook URL"},
		{"notifications discord status [--project name]", "show Discord notification state"},
		{"notifications discord test [--project name]", "send a test Discord notification"},
		{"notifications discord enable [--project name]", "enable Discord notifications"},
		{"notifications discord disable [--project name]", "disable Discord notifications"},
		{"notifications discord remove [--project name]", "delete the Discord webhook secret"},
	}},
	{"System", []commandHelp{
		{"daemon", "run the webhook daemon"},
		{"version", "print build information"},
		{"help", "show this help"},
	}},
}

func (a App) usage() error {
	fmt.Fprintln(a.Out, "Deploy Agent")
	fmt.Fprintln(a.Out, "Receive signed GitHub push webhooks and deploy registered repositories.")
	fmt.Fprintln(a.Out)
	fmt.Fprintln(a.Out, "Usage:")
	fmt.Fprintln(a.Out, "  deploy <command> [options]")
	fmt.Fprintln(a.Out)
	fmt.Fprintln(a.Out, "Commands:")
	for _, group := range commandGroups {
		fmt.Fprintf(a.Out, "\n%s:\n", group.Title)
		rows := make([][]string, 0, len(group.Commands))
		for _, cmd := range group.Commands {
			rows = append(rows, []string{"  " + cmd.Usage, cmd.Description})
		}
		writeTable(a.Out, rows)
	}
	fmt.Fprintln(a.Out)
	fmt.Fprintln(a.Out, "Examples:")
	fmt.Fprintln(a.Out, "  sudo deploy install --user deploy --listen 127.0.0.1 --port 4747 --public-url https://deploy.example.com")
	fmt.Fprintln(a.Out, "  deploy init")
	fmt.Fprintln(a.Out, "  deploy run --project flux --dry-run")
	fmt.Fprintln(a.Out, "  deploy webhook show --project flux")
	return nil
}

func writeSection(w io.Writer, title string) {
	fmt.Fprintf(w, "%s\n%s\n", title, strings.Repeat("-", len(title)))
}

func writeFields(w io.Writer, fields ...[2]string) {
	width := 0
	for _, f := range fields {
		if len(f[0])+1 > width {
			width = len(f[0]) + 1
		}
	}
	for _, f := range fields {
		fmt.Fprintf(w, "%-*s  %s\n", width, f[0]+":", emptyValue(f[1]))
	}
}

func writeTable(w io.Writer, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	_ = tw.Flush()
}

func formatBool(v bool) string {
	if v {
		return "enabled"
	}
	return "disabled"
}

func formatCheck(ok bool) string {
	if ok {
		return "[ok]"
	}
	return "[fail]"
}

func formatCommit(v string) string {
	if v == "" {
		return "-"
	}
	return short(v)
}

func formatTime(v time.Time) string {
	if v.IsZero() {
		return "-"
	}
	return v.Format(time.RFC3339)
}

func formatDuration(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	return time.Duration(ms * int64(time.Millisecond)).Round(time.Second).String()
}

func emptyValue(v string) string {
	if v == "" {
		return "-"
	}
	return v
}

func maskSecret(secret string) string {
	if len(secret) <= 12 {
		return strings.Repeat("*", len(secret))
	}
	return secret[:8] + "..." + secret[len(secret)-4:]
}
