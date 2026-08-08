package plugin

import "strings"

func ExpandPluginVariables(value, pluginRoot, pluginData string) string {
	const rootToken = "${PLUGIN_ROOT}"
	const dataToken = "${PLUGIN_DATA}"
	var expanded strings.Builder
	for len(value) > 0 {
		rootIndex := strings.Index(value, rootToken)
		dataIndex := strings.Index(value, dataToken)
		index, token, replacement := rootIndex, rootToken, pluginRoot
		if index < 0 || (dataIndex >= 0 && dataIndex < index) {
			index, token, replacement = dataIndex, dataToken, pluginData
		}
		if index < 0 {
			expanded.WriteString(value)
			break
		}
		expanded.WriteString(value[:index])
		expanded.WriteString(replacement)
		value = value[index+len(token):]
	}
	return expanded.String()
}
