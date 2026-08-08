package contentcloud

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

//go:embed plugins/contentcloud-video-production
var agentPlugins embed.FS

func AgentPlugin(name string) (fs.FS, error) {
	if strings.TrimSpace(name) != "contentcloud-video-production" {
		return nil, fmt.Errorf("Agent Plugin %q is not bundled", name)
	}
	return fs.Sub(agentPlugins, "plugins/"+name)
}
