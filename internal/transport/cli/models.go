package cli

import (
	"github.com/spf13/cobra"

	modelprovider "github.com/limecloud/contentcloud/internal/integration/provider/model"

	"github.com/limecloud/contentcloud/internal/application"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
)

func (r *Root) modelCommand() *cobra.Command {
	command := &cobra.Command{Use: "model", Short: "通过可替换 Provider 生成受治理内容候选"}
	command.AddCommand(r.simpleUserReadCommand("providers", "model.provider.list", "列出已配置 vLLM、SGLang 等 Provider", map[string]any{}))

	var providerID, systemPrompt, prompt, contentType, schemaVersion string
	var maxTokens int
	generate := &cobra.Command{Use: "generate <task-id>", Args: cobra.ExactArgs(1), Short: "生成 draft TaskRevision，不绕过审核", RunE: func(cmd *cobra.Command, args []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		messages := []modelprovider.Message{}
		if systemPrompt != "" {
			messages = append(messages, modelprovider.Message{Role: "system", Content: systemPrompt})
		}
		messages = append(messages, modelprovider.Message{Role: "user", Content: prompt})
		input := application.GenerateModelCandidateInput{ProviderID: providerID, Messages: messages, ResponseSchema: map[string]any{"type": "object", "additionalProperties": true}, ContentType: contentType, SchemaVersion: schemaVersion, MaxTokens: maxTokens}
		params := struct {
			TaskID string `json:"task_id"`
			application.GenerateModelCandidateInput
		}{TaskID: args[0], GenerateModelCandidateInput: input}
		var result application.GenerateModelCandidateResult
		if err := client.Dispatch(cmd.Context(), "model.candidate.generate", params, &result); err != nil {
			return err
		}
		return r.writeOK("model.candidate.generate", result)
	}}
	generate.Flags().StringVar(&providerID, "provider", "", "Provider ID，例如 vllm 或 sglang")
	generate.Flags().StringVar(&systemPrompt, "system", "", "可选系统约束")
	generate.Flags().StringVar(&prompt, "prompt", "", "候选生成任务")
	generate.Flags().StringVar(&contentType, "content-type", "", "候选内容类型，默认使用任务类型")
	generate.Flags().StringVar(&schemaVersion, "schema-version", "", "候选业务 Schema 版本")
	generate.Flags().IntVar(&maxTokens, "max-tokens", 4096, "最大输出 Token")
	_ = generate.MarkFlagRequired("provider")
	_ = generate.MarkFlagRequired("prompt")
	_ = generate.MarkFlagRequired("schema-version")

	var taskID string
	receipts := &cobra.Command{Use: "receipts", Short: "列出模型生成摘要和 Token 用量", RunE: func(cmd *cobra.Command, _ []string) error {
		_, client, _, err := r.userClient()
		if err != nil {
			return err
		}
		var result []deliverydomain.ModelGenerationReceipt
		if err := client.Dispatch(cmd.Context(), "model.receipt.list", map[string]any{"task_id": taskID}, &result); err != nil {
			return err
		}
		return r.writeOK("model.receipt.list", result)
	}}
	receipts.Flags().StringVar(&taskID, "task", "", "任务 ID")
	_ = receipts.MarkFlagRequired("task")
	command.AddCommand(generate, receipts)
	return command
}
