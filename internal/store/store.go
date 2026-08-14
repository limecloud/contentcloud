package store

import (
	"context"
	"time"

	"github.com/limecloud/contentcloud/internal/domain"
)

type Store interface {
	CreateUser(context.Context, domain.User) error
	UserByEmail(context.Context, string) (domain.User, error)
	UserByID(context.Context, string) (domain.User, error)
	SaveUser(context.Context, domain.User) error
	SaveSession(context.Context, domain.Session) error
	SessionByID(context.Context, string) (domain.Session, error)
	RevokeSession(context.Context, string, time.Time) error
	RevokeSessionsForUserTenant(context.Context, string, string, time.Time) error

	CreateTenant(context.Context, domain.Tenant, domain.Membership) error
	TenantsForUser(context.Context, string) ([]domain.Tenant, error)
	Membership(context.Context, string, string) (domain.Membership, error)
	Memberships(context.Context, string) ([]domain.Membership, error)
	SaveMembership(context.Context, domain.Membership) error
	CreateMembershipInvite(context.Context, domain.MembershipInvite) error
	MembershipInviteByTokenHash(context.Context, string) (domain.MembershipInvite, error)
	MembershipInvites(context.Context, string) ([]domain.MembershipInvite, error)
	SaveMembershipInvite(context.Context, domain.MembershipInvite) error
	AcceptMembershipInvite(context.Context, string, domain.User, time.Time) (domain.Membership, error)
	RegisterWithInvite(context.Context, domain.User, string, domain.Session, time.Time) (domain.Session, domain.Membership, error)
	PlatformTenants(context.Context) ([]domain.PlatformTenant, error)
	PlatformUsers(context.Context) ([]domain.PlatformUser, error)
	SetTenantStatus(context.Context, string, string, time.Time) (domain.Tenant, error)
	TenantContentCapabilities(context.Context, string) ([]domain.TenantContentCapability, error)
	SetTenantContentCapability(context.Context, domain.TenantContentCapability) error

	CreateProject(context.Context, domain.Project) error
	Projects(context.Context, string) ([]domain.Project, error)
	Project(context.Context, string, string) (domain.Project, error)
	SaveProject(context.Context, domain.Project) error
	UpdateProject(context.Context, domain.Project, int) error
	CreateProjectTemplate(context.Context, domain.ProjectTemplate) error
	ProjectTemplates(context.Context, string) ([]domain.ProjectTemplate, error)
	ProjectTemplate(context.Context, string, string) (domain.ProjectTemplate, error)

	CreateConnectSession(context.Context, domain.ConnectSession) error
	ConnectSessionByID(context.Context, string, string) (domain.ConnectSession, error)
	SaveConnectSession(context.Context, domain.ConnectSession) error
	CreateBootstrapAttemptForSession(context.Context, string, domain.BootstrapAttempt, time.Time) (domain.BootstrapAttempt, error)
	BootstrapAttempt(context.Context, string, string) (domain.BootstrapAttempt, error)
	BootstrapAttemptByTokenHash(context.Context, string) (domain.BootstrapAttempt, error)
	ApproveBootstrapAttempt(context.Context, string, string, string, string, time.Time) (domain.BootstrapAttempt, error)
	DenyBootstrapAttempt(context.Context, string, string, string, string, time.Time) (domain.BootstrapAttempt, error)
	AppendBootstrapProgress(context.Context, string, domain.BootstrapProgressEvent, time.Time) (domain.BootstrapProgressEvent, error)
	BootstrapProgressForSession(context.Context, string, string) (*domain.BootstrapProgress, error)
	ConsumeBootstrapAttempt(context.Context, string, domain.Device, domain.WorkspaceBinding, time.Time) (domain.ConnectSession, domain.BootstrapAttempt, domain.Device, domain.WorkspaceBinding, error)
	CompleteBootstrapAttempt(context.Context, string, string, time.Time) (domain.BootstrapAttempt, error)
	CreateBootstrapDiagnostic(context.Context, domain.BootstrapDiagnostic) (domain.BootstrapDiagnostic, error)
	SaveDevice(context.Context, domain.Device) error
	DeviceByTokenHash(context.Context, string) (domain.Device, error)
	Devices(context.Context, string, string) ([]domain.Device, error)
	Device(context.Context, string, string) (domain.Device, error)
	GrantDeviceProject(context.Context, string, string, string, string, time.Time) error
	RevokeDeviceProject(context.Context, string, string, string, time.Time) error
	RevokeDevice(context.Context, string, string, time.Time) error
	CreateWorkspaceBinding(context.Context, domain.WorkspaceBinding) error
	WorkspaceBindingByTokenHash(context.Context, string) (domain.WorkspaceBinding, error)
	WorkspaceBinding(context.Context, string, string) (domain.WorkspaceBinding, error)
	SaveWorkspaceBinding(context.Context, domain.WorkspaceBinding) error

	CreateUserDeviceFlow(context.Context, domain.UserDeviceFlow) error
	UserDeviceFlowByCodeHash(context.Context, string) (domain.UserDeviceFlow, error)
	UserDeviceFlowByUserCode(context.Context, string) (domain.UserDeviceFlow, error)
	SaveUserDeviceFlow(context.Context, domain.UserDeviceFlow) error
	CreateCLIToken(context.Context, domain.CLIToken) error
	CLITokenByHash(context.Context, string) (domain.CLIToken, error)
	RevokeCLIToken(context.Context, string, time.Time) error

	CreateSource(context.Context, domain.Source, domain.SourceRevision) error
	CreateSourceRevision(context.Context, domain.SourceRevision) error
	Sources(context.Context, string, string) ([]domain.Source, error)
	Source(context.Context, string, string) (domain.Source, error)
	SourceRevisions(context.Context, string, string) ([]domain.SourceRevision, error)
	SourceRevision(context.Context, string, string) (domain.SourceRevision, error)
	SaveSourceRevision(context.Context, domain.SourceRevision) error
	PendingSourceRevisions(context.Context, int) ([]domain.SourceRevision, error)
	ClaimSourceRevision(context.Context, string, string) (domain.SourceRevision, bool, error)
	CreateEvidence(context.Context, domain.EvidenceSpan) error
	Evidence(context.Context, string, string) ([]domain.EvidenceSpan, error)
	EvidenceSpan(context.Context, string, string) (domain.EvidenceSpan, error)
	SaveEvidence(context.Context, domain.EvidenceSpan) error
	CreateAsset(context.Context, domain.Asset) error
	Assets(context.Context, string, string) ([]domain.Asset, error)
	Asset(context.Context, string, string) (domain.Asset, error)
	SaveAsset(context.Context, domain.Asset) error
	CreateWorkspaceFolder(context.Context, domain.WorkspaceFolder) error
	WorkspaceFolders(context.Context, string, string) ([]domain.WorkspaceFolder, error)
	WorkspaceFolder(context.Context, string, string) (domain.WorkspaceFolder, error)
	CreateWorkspaceMaterial(context.Context, domain.WorkspaceMaterial) error
	WorkspaceMaterials(context.Context, string, string) ([]domain.WorkspaceMaterial, error)
	WorkspaceMaterial(context.Context, string, string) (domain.WorkspaceMaterial, error)
	SaveWorkspaceMaterial(context.Context, domain.WorkspaceMaterial) error
	CreateRightsRecord(context.Context, domain.RightsRecord) error
	RightsRecords(context.Context, string, string) ([]domain.RightsRecord, error)
	RightsRecord(context.Context, string, string) (domain.RightsRecord, error)
	SaveRightsRecord(context.Context, domain.RightsRecord) error

	CreateKnowledgeObject(context.Context, domain.KnowledgeObject) error
	CreateKnowledgeObjectDecision(context.Context, domain.KnowledgeObject, domain.KnowledgeDecision) error
	KnowledgeObjects(context.Context, string, string) ([]domain.KnowledgeObject, error)
	KnowledgeObject(context.Context, string, string, int) (domain.KnowledgeObject, error)
	CreateKnowledgePack(context.Context, domain.KnowledgePack) error
	KnowledgePacks(context.Context, string, string) ([]domain.KnowledgePack, error)
	KnowledgePack(context.Context, string, string) (domain.KnowledgePack, error)
	SaveKnowledgePack(context.Context, domain.KnowledgePack) error
	CreateKnowledgeSnapshot(context.Context, domain.KnowledgeSnapshot) error
	KnowledgeSnapshots(context.Context, string, string, string) ([]domain.KnowledgeSnapshot, error)
	KnowledgeSnapshot(context.Context, string, string) (domain.KnowledgeSnapshot, error)
	KnowledgeDecisions(context.Context, string, string) ([]domain.KnowledgeDecision, error)

	CreateEnvironment(context.Context, domain.Environment) error
	Environments(context.Context, string) ([]domain.Environment, error)
	Environment(context.Context, string, string) (domain.Environment, error)
	SaveEnvironment(context.Context, domain.Environment) error
	CreateSOP(context.Context, domain.SOPDefinition, domain.SOPVersion) error
	SaveSOPDefinition(context.Context, domain.SOPDefinition) error
	CreateSOPVersion(context.Context, domain.SOPVersion) error
	SOPs(context.Context, string) ([]domain.SOPSummary, error)
	SOP(context.Context, string, string) (domain.SOPSummary, error)
	SaveSOPVersion(context.Context, domain.SOPVersion) error
	PublishSOPVersion(context.Context, string, string, int, string, time.Time) (domain.SOPVersion, error)
	RetireSOPVersion(context.Context, string, string, int, time.Time) error
	SaveProjectSOPBinding(context.Context, domain.ProjectSOPBinding) error
	ProjectSOPBinding(context.Context, string, string) (domain.ProjectSOPBinding, error)
	ProjectSOPBindings(context.Context, string) ([]domain.ProjectSOPBinding, error)
	CreateWorkTask(context.Context, domain.WorkTask) error
	WorkTaskByIdempotencyKey(context.Context, string, string) (domain.WorkTask, error)
	WorkTasks(context.Context, string, string) ([]domain.WorkTask, error)
	WorkTask(context.Context, string, string) (domain.WorkTask, error)
	SaveWorkTask(context.Context, domain.WorkTask) error
	CreateInputItem(context.Context, domain.InputItem) error
	InputItems(context.Context, string, string, string, string) ([]domain.InputItem, error)
	InputItem(context.Context, string, string) (domain.InputItem, error)
	InputItemByIdempotencyKey(context.Context, string, string) (domain.InputItem, error)
	SaveInputItem(context.Context, domain.InputItem, int) error
	CreateConversationImport(context.Context, domain.ConversationImport) error
	ConversationImport(context.Context, string, string) (domain.ConversationImport, error)
	ConversationImportByIdempotencyKey(context.Context, string, string) (domain.ConversationImport, error)
	ConversationImportsForTask(context.Context, string, string) ([]domain.ConversationImport, error)
	SaveConversationImport(context.Context, domain.ConversationImport) error
	StageRuns(context.Context, string, string) ([]domain.StageRun, error)
	CreateStageRun(context.Context, domain.StageRun) error
	SaveStageRun(context.Context, domain.StageRun) error
	CompleteStageRun(context.Context, domain.StageRun, []domain.TaskStageOutput) error
	TaskStageOutputs(context.Context, string, string) ([]domain.TaskStageOutput, error)
	CreateProviderProfile(context.Context, domain.ProviderProfile) error
	ProviderProfile(context.Context, string, string) (domain.ProviderProfile, error)
	SaveProviderBinding(context.Context, domain.ProviderBinding) error
	ProviderBinding(context.Context, string, string) (domain.ProviderBinding, error)
	CreateMediaGenerationJob(context.Context, domain.MediaGenerationJob) error
	PendingMediaGenerationJobs(context.Context, int) ([]domain.MediaGenerationJob, error)
	MediaGenerationJob(context.Context, string, string) (domain.MediaGenerationJob, error)
	MediaGenerationJobs(context.Context, string, string) ([]domain.MediaGenerationJob, error)
	SaveMediaGenerationJob(context.Context, domain.MediaGenerationJob, int) error
	CreateProviderAttempt(context.Context, domain.ProviderAttempt) error
	SaveProviderAttempt(context.Context, domain.ProviderAttempt) error
	ProviderAttempts(context.Context, string, string) ([]domain.ProviderAttempt, error)
	ProviderAttemptsByRuntimeJob(context.Context, string, string) ([]domain.ProviderAttempt, error)
	CreateMediaReview(context.Context, domain.MediaReview) error
	SaveMediaReview(context.Context, domain.MediaReview, int) error
	MediaReview(context.Context, string, string) (domain.MediaReview, error)
	MediaReviews(context.Context, string, string) ([]domain.MediaReview, error)
	CreateGateEvaluation(context.Context, domain.GateEvaluation) error
	GateEvaluations(context.Context, string, string) ([]domain.GateEvaluation, error)
	GateEvaluation(context.Context, string, string) (domain.GateEvaluation, error)
	SaveGateEvaluation(context.Context, domain.GateEvaluation) error
	CreateTaskRevision(context.Context, domain.TaskRevision) error
	TaskRevisions(context.Context, string, string) ([]domain.TaskRevision, error)
	TaskRevision(context.Context, string, string) (domain.TaskRevision, error)
	CreateTaskDelivery(context.Context, domain.TaskDelivery) error
	TaskDeliveries(context.Context, string, string) ([]domain.TaskDelivery, error)
	TaskDelivery(context.Context, string, string) (domain.TaskDelivery, error)
	SaveTaskDelivery(context.Context, domain.TaskDelivery) error
	CreateChannelBinding(context.Context, domain.ChannelBinding) error
	ChannelBindings(context.Context, string, string) ([]domain.ChannelBinding, error)
	ChannelBinding(context.Context, string, string) (domain.ChannelBinding, error)
	SaveChannelBinding(context.Context, domain.ChannelBinding) error
	CreateChannelPublication(context.Context, domain.ChannelPublication) error
	ChannelPublicationByIdempotencyKey(context.Context, string, string) (domain.ChannelPublication, error)
	ChannelPublications(context.Context, string, string) ([]domain.ChannelPublication, error)
	ChannelPublication(context.Context, string, string) (domain.ChannelPublication, error)
	SaveChannelPublication(context.Context, domain.ChannelPublication) error
	ApplyChannelCallback(context.Context, domain.ChannelPublication, domain.ChannelCallbackReceipt) (bool, error)
	CreateModelGenerationReceipt(context.Context, domain.ModelGenerationReceipt) error
	CreateModelGeneratedRevision(context.Context, domain.TaskRevision, domain.ModelGenerationReceipt) error
	ModelGenerationReceipts(context.Context, string, string) ([]domain.ModelGenerationReceipt, error)

	CreateSnapshot(context.Context, domain.ContextSnapshot) error
	Snapshot(context.Context, string, string) (domain.ContextSnapshot, error)

	CreateApproval(context.Context, domain.ApprovalDecision) error
	Approvals(context.Context, string, string) ([]domain.ApprovalDecision, error)
	CreateReviewCycle(context.Context, domain.ReviewCycle) (domain.ReviewCycle, error)
	ReviewCycles(context.Context, string, string) ([]domain.ReviewCycle, error)
	SaveReviewCycle(context.Context, domain.ReviewCycle) error
	CreateReviewComment(context.Context, domain.ReviewComment) error
	ReviewComments(context.Context, string, string) ([]domain.ReviewComment, error)
	ReviewComment(context.Context, string, string) (domain.ReviewComment, error)
	SaveReviewComment(context.Context, domain.ReviewComment) error
	CreateReviewGrant(context.Context, domain.ReviewGrant) error
	ReviewGrant(context.Context, string, string) (domain.ReviewGrant, error)
	ReviewGrants(context.Context, string, string) ([]domain.ReviewGrant, error)
	ReviewGrantByTokenHash(context.Context, string) (domain.ReviewGrant, error)
	MarkReviewGrantVerified(context.Context, string, string, time.Time) error
	RevokeReviewGrant(context.Context, string, string, time.Time) error
	CreateSubmissionRevision(context.Context, domain.Submission, domain.SubmissionRevision, []domain.SourceDisclosure, domain.ReviewCycle) error
	SubmissionByWorkspaceType(context.Context, string, string, string, string) (domain.Submission, error)
	Submissions(context.Context, string, string) ([]domain.Submission, error)
	Submission(context.Context, string, string) (domain.Submission, error)
	SubmissionRevision(context.Context, string, string) (domain.SubmissionRevision, error)
	SubmissionRevisions(context.Context, string, string) ([]domain.SubmissionRevision, error)
	ApprovedSnapshots(context.Context, string, string, string) ([]domain.ApprovedSnapshot, error)
	ApprovedSnapshot(context.Context, string, string) (domain.ApprovedSnapshot, error)
	RecordSubmissionApproval(context.Context, domain.Submission, domain.ApprovalDecision) error
	ApproveSubmissionRevision(context.Context, domain.Submission, domain.ApprovedSnapshot, domain.ApprovalDecision) error
	RequestSubmissionChanges(context.Context, domain.Submission, domain.ApprovalDecision, domain.ReviewComment) error
	CreateSubmissionReviewGrant(context.Context, domain.Submission, domain.ReviewGrant) error
	CompleteSubmissionClientReview(context.Context, domain.Submission, domain.ReviewGrant, domain.ApprovalDecision, *domain.ReviewComment, *domain.ApprovedSnapshot) error

	CreateArtifact(context.Context, domain.Artifact) error
	ArtifactsByApprovedSnapshot(context.Context, string, string) ([]domain.Artifact, error)
	Artifact(context.Context, string, string) (domain.Artifact, error)
	CreateDeliveryPackage(context.Context, domain.DeliveryPackage, []domain.Artifact) error
	DeliveryPackages(context.Context, string, string) ([]domain.DeliveryPackage, error)
	DeliveryPackage(context.Context, string, string) (domain.DeliveryPackage, error)
	CreatePerformanceImportBatch(context.Context, domain.PerformanceImportBatch, []domain.PerformanceObservation) error
	PerformanceImportBatches(context.Context, string, string) ([]domain.PerformanceImportBatch, error)
	PerformanceImportBatch(context.Context, string, string) (domain.PerformanceImportBatch, error)
	ExistingPerformanceDedupKeys(context.Context, string, string, []string) (map[string]string, error)
	PerformanceObservation(context.Context, string, string) (domain.PerformanceObservation, error)
	PerformanceObservations(context.Context, string, string) ([]domain.PerformanceObservation, error)
	CreateRatingDecision(context.Context, domain.RatingDecision) error
	RatingDecisions(context.Context, string, string) ([]domain.RatingDecision, error)

	AppendAudit(context.Context, domain.AuditEvent) error
	AuditEvents(context.Context, string, string, int) ([]domain.AuditEvent, error)
}
