package cli

import (
	"time"

	"github.com/spf13/cobra"

	localworkspace "github.com/limecloud/contentcloud/internal/local/workspace"
)

func (r *Root) novelCommand() *cobra.Command {
	command := &cobra.Command{Use: "novel", Short: "校验并演进小说 Canon，生成可发布章节包"}
	var root, canonFile, chapterFile string
	lint := &cobra.Command{Use: "lint", Short: "用持久化 Canon 校验章节连续性", RunE: func(cmd *cobra.Command, _ []string) error {
		report, err := localworkspace.LintNovelContinuityFiles(root, canonFile, chapterFile)
		if err != nil {
			return err
		}
		return r.writeOK("novel.lint", report)
	}}
	bindNovelFiles(lint, &root, &canonFile, &chapterFile)

	var outputFile string
	apply := &cobra.Command{Use: "canon-apply", Short: "连续性通过后确定性更新 Canon 版本、伏笔和时间线", RunE: func(cmd *cobra.Command, _ []string) error {
		canon, err := localworkspace.ApplyNovelChapterFiles(root, canonFile, chapterFile, outputFile)
		if err != nil {
			return err
		}
		return r.writeOK("novel.canon.apply", canon)
	}}
	bindNovelFiles(apply, &root, &canonFile, &chapterFile)
	apply.Flags().StringVar(&outputFile, "output", "", "新 Canon 文件；为空时原子替换当前 Canon")

	var projectID, outputDirectory, channelProfile string
	pack := &cobra.Command{Use: "package", Short: "从 ApprovedSnapshot 章节和 Canon 生成渠道发布包", RunE: func(cmd *cobra.Command, _ []string) error {
		result, err := localworkspace.BuildNovelRelease(localworkspace.BuildNovelReleaseOptions{Root: root, ProjectID: projectID, CanonFile: canonFile, ChapterFile: chapterFile, OutputDirectory: outputDirectory, ChannelProfileRef: channelProfile, Now: time.Now().UTC()})
		if err != nil {
			return err
		}
		return r.writeOK("novel.package", result)
	}}
	bindNovelFiles(pack, &root, &canonFile, &chapterFile)
	pack.Flags().StringVar(&projectID, "project", "", "项目 ID")
	pack.Flags().StringVar(&outputDirectory, "output", "", "工作区内交付目录")
	pack.Flags().StringVar(&channelProfile, "channel-profile", "channel:web-novel@1.0.0", "渠道 Profile 引用")
	_ = pack.MarkFlagRequired("project")
	command.AddCommand(lint, apply, pack)
	return command
}

func bindNovelFiles(command *cobra.Command, root, canonFile, chapterFile *string) {
	command.Flags().StringVar(root, "directory", ".", "ContentCloud 工作区")
	command.Flags().StringVar(canonFile, "canon", "", "Canon JSON 文件")
	command.Flags().StringVar(chapterFile, "chapter", "", "章节 JSON 文件")
	_ = command.MarkFlagRequired("canon")
	_ = command.MarkFlagRequired("chapter")
}
