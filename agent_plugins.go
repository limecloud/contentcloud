package contentcloud

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed plugins/contentcloud-video-production plugins/contentcloud-wechat-article plugins/contentcloud-marketing
var agentPlugins embed.FS

func AgentPlugin(name string) (fs.FS, error) {
	if name != "contentcloud-video-production" && name != "contentcloud-wechat-article" && name != "contentcloud-marketing" {
		return nil, fmt.Errorf("Agent Plugin %q is not bundled", name)
	}
	return fs.Sub(agentPlugins, "plugins/"+name)
}
