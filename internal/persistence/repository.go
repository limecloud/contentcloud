package persistence

import (
	"context"
	"time"

	auditdomain "github.com/limecloud/contentcloud/internal/audit"
	catalogdomain "github.com/limecloud/contentcloud/internal/catalog"
	deliverydomain "github.com/limecloud/contentcloud/internal/delivery"
	identitydomain "github.com/limecloud/contentcloud/internal/identity"
	performancedomain "github.com/limecloud/contentcloud/internal/performance"
	reviewdomain "github.com/limecloud/contentcloud/internal/review"
	sourcedomain "github.com/limecloud/contentcloud/internal/source"
	"github.com/limecloud/contentcloud/internal/work"
	workspacedomain "github.com/limecloud/contentcloud/internal/workspace"
)

type IdentityRepository interface {
	CreateUser(context.Context, identitydomain.User) error
	UserByEmail(context.Context, string) (identitydomain.User, error)
	UserByID(context.Context, string) (identitydomain.User, error)
	SaveUser(context.Context, identitydomain.User) error
	SaveSession(context.Context, identitydomain.Session) error
	SessionByID(context.Context, string) (identitydomain.Session, error)
	RevokeSession(context.Context, string, time.Time) error
	RevokeSessionsForUserTenant(context.Context, string, string, time.Time) error

	CreateTenant(context.Context, identitydomain.Tenant, identitydomain.Membership) error
	TenantsForUser(context.Context, string) ([]identitydomain.Tenant, error)
	Membership(context.Context, string, string) (identitydomain.Membership, error)
	Memberships(context.Context, string) ([]identitydomain.Membership, error)
	SaveMembership(context.Context, identitydomain.Membership) error
	CreateMembershipInvite(context.Context, identitydomain.MembershipInvite) error
	MembershipInviteByTokenHash(context.Context, string) (identitydomain.MembershipInvite, error)
	MembershipInvites(context.Context, string) ([]identitydomain.MembershipInvite, error)
	SaveMembershipInvite(context.Context, identitydomain.MembershipInvite) error
	AcceptMembershipInvite(context.Context, string, identitydomain.User, time.Time) (identitydomain.Membership, error)
	RegisterWithInvite(context.Context, identitydomain.User, string, identitydomain.Session, time.Time) (identitydomain.Session, identitydomain.Membership, error)
	PlatformTenants(context.Context) ([]identitydomain.PlatformTenant, error)
	PlatformUsers(context.Context) ([]identitydomain.PlatformUser, error)
	SetTenantStatus(context.Context, string, string, time.Time) (identitydomain.Tenant, error)
	TenantContentCapabilities(context.Context, string) ([]identitydomain.TenantContentCapability, error)
	SetTenantContentCapability(context.Context, identitydomain.TenantContentCapability) error
}

type WorkspaceRepository interface {
	CreateProject(context.Context, workspacedomain.Project) error
	Projects(context.Context, string) ([]workspacedomain.Project, error)
	Project(context.Context, string, string) (workspacedomain.Project, error)
	SaveProject(context.Context, workspacedomain.Project) error
	UpdateProject(context.Context, workspacedomain.Project, int) error
	CreateProjectTemplate(context.Context, workspacedomain.ProjectTemplate) error
	ProjectTemplates(context.Context, string) ([]workspacedomain.ProjectTemplate, error)
	ProjectTemplate(context.Context, string, string) (workspacedomain.ProjectTemplate, error)

	CreateConnectSession(context.Context, workspacedomain.ConnectSession) error
	ConnectSessionByID(context.Context, string, string) (workspacedomain.ConnectSession, error)
	SaveConnectSession(context.Context, workspacedomain.ConnectSession) error
	CreateBootstrapAttemptForSession(context.Context, string, workspacedomain.BootstrapAttempt, time.Time) (workspacedomain.BootstrapAttempt, error)
	BootstrapAttempt(context.Context, string, string) (workspacedomain.BootstrapAttempt, error)
	BootstrapAttemptByTokenHash(context.Context, string) (workspacedomain.BootstrapAttempt, error)
	ApproveBootstrapAttempt(context.Context, string, string, string, string, time.Time) (workspacedomain.BootstrapAttempt, error)
	DenyBootstrapAttempt(context.Context, string, string, string, string, time.Time) (workspacedomain.BootstrapAttempt, error)
	AppendBootstrapProgress(context.Context, string, workspacedomain.BootstrapProgressEvent, time.Time) (workspacedomain.BootstrapProgressEvent, error)
	BootstrapProgressForSession(context.Context, string, string) (*workspacedomain.BootstrapProgress, error)
	ConsumeBootstrapAttempt(context.Context, string, workspacedomain.Device, workspacedomain.WorkspaceBinding, time.Time) (workspacedomain.ConnectSession, workspacedomain.BootstrapAttempt, workspacedomain.Device, workspacedomain.WorkspaceBinding, error)
	CompleteBootstrapAttempt(context.Context, string, string, time.Time) (workspacedomain.BootstrapAttempt, error)
	CreateBootstrapDiagnostic(context.Context, workspacedomain.BootstrapDiagnostic) (workspacedomain.BootstrapDiagnostic, error)
	SaveDevice(context.Context, workspacedomain.Device) error
	DeviceByTokenHash(context.Context, string) (workspacedomain.Device, error)
	Devices(context.Context, string, string) ([]workspacedomain.Device, error)
	Device(context.Context, string, string) (workspacedomain.Device, error)
	GrantDeviceProject(context.Context, string, string, string, string, time.Time) error
	RevokeDeviceProject(context.Context, string, string, string, time.Time) error
	RevokeDevice(context.Context, string, string, time.Time) error
	CreateWorkspaceBinding(context.Context, workspacedomain.WorkspaceBinding) error
	WorkspaceBindingByTokenHash(context.Context, string) (workspacedomain.WorkspaceBinding, error)
	WorkspaceBinding(context.Context, string, string) (workspacedomain.WorkspaceBinding, error)
	SaveWorkspaceBinding(context.Context, workspacedomain.WorkspaceBinding) error
	PublishWorkspaceRevision(context.Context, workspacedomain.WorkspaceRevision) (workspacedomain.WorkspaceRevision, error)
	LatestWorkspaceRevision(context.Context, string, string) (workspacedomain.WorkspaceRevision, error)
	WorkspaceRevisionsAfter(context.Context, string, string, int64, int) ([]workspacedomain.WorkspaceRevision, error)
	CreateWorkspaceUploadSession(context.Context, workspacedomain.WorkspaceUploadSession) (workspacedomain.WorkspaceUploadSession, error)
	WorkspaceUploadSession(context.Context, string, string) (workspacedomain.WorkspaceUploadSession, error)
	SaveWorkspaceUploadPart(context.Context, string, workspacedomain.WorkspaceUploadPart) (workspacedomain.WorkspaceUploadPart, error)
	WorkspaceUploadParts(context.Context, string, string) ([]workspacedomain.WorkspaceUploadPart, error)
	CompleteWorkspaceUpload(context.Context, workspacedomain.WorkspaceUploadSession, workspacedomain.WorkspaceObject) error
	WorkspaceObject(context.Context, string, string, string) (workspacedomain.WorkspaceObject, error)
	WorkspaceObjects(context.Context, string, string, []string) ([]workspacedomain.WorkspaceObject, error)

	CreateUserDeviceFlow(context.Context, workspacedomain.UserDeviceFlow) error
	UserDeviceFlowByCodeHash(context.Context, string) (workspacedomain.UserDeviceFlow, error)
	UserDeviceFlowByUserCode(context.Context, string) (workspacedomain.UserDeviceFlow, error)
	SaveUserDeviceFlow(context.Context, workspacedomain.UserDeviceFlow) error
	CreateCLIToken(context.Context, workspacedomain.CLIToken) error
	CLITokenByHash(context.Context, string) (workspacedomain.CLIToken, error)
	RevokeCLIToken(context.Context, string, time.Time) error
	CreateWorkspaceFolder(context.Context, workspacedomain.WorkspaceFolder) error
	WorkspaceFolders(context.Context, string, string) ([]workspacedomain.WorkspaceFolder, error)
	WorkspaceFolder(context.Context, string, string) (workspacedomain.WorkspaceFolder, error)
	CreateWorkspaceMaterial(context.Context, workspacedomain.WorkspaceMaterial) error
	WorkspaceMaterials(context.Context, string, string) ([]workspacedomain.WorkspaceMaterial, error)
	WorkspaceMaterial(context.Context, string, string) (workspacedomain.WorkspaceMaterial, error)
	SaveWorkspaceMaterial(context.Context, workspacedomain.WorkspaceMaterial) error
}

type SourceRepository interface {
	CreateSource(context.Context, sourcedomain.Source, sourcedomain.SourceRevision) error
	CreateSourceRevision(context.Context, sourcedomain.SourceRevision) error
	Sources(context.Context, string, string) ([]sourcedomain.Source, error)
	Source(context.Context, string, string) (sourcedomain.Source, error)
	SourceRevisions(context.Context, string, string) ([]sourcedomain.SourceRevision, error)
	SourceRevision(context.Context, string, string) (sourcedomain.SourceRevision, error)
	SaveSourceRevision(context.Context, sourcedomain.SourceRevision) error
	PendingSourceRevisions(context.Context, int) ([]sourcedomain.SourceRevision, error)
	ClaimSourceRevision(context.Context, string, string) (sourcedomain.SourceRevision, bool, error)
	CreateEvidence(context.Context, sourcedomain.EvidenceSpan) error
	Evidence(context.Context, string, string) ([]sourcedomain.EvidenceSpan, error)
	EvidenceSpan(context.Context, string, string) (sourcedomain.EvidenceSpan, error)
	SaveEvidence(context.Context, sourcedomain.EvidenceSpan) error
	CreateAsset(context.Context, sourcedomain.Asset) error
	Assets(context.Context, string, string) ([]sourcedomain.Asset, error)
	Asset(context.Context, string, string) (sourcedomain.Asset, error)
	SaveAsset(context.Context, sourcedomain.Asset) error
	CreateRightsRecord(context.Context, sourcedomain.RightsRecord) error
	RightsRecords(context.Context, string, string) ([]sourcedomain.RightsRecord, error)
	RightsRecord(context.Context, string, string) (sourcedomain.RightsRecord, error)
	SaveRightsRecord(context.Context, sourcedomain.RightsRecord) error
}

type KnowledgeRepository interface {
	CreateKnowledgeObject(context.Context, sourcedomain.KnowledgeObject) error
	CreateKnowledgeObjectDecision(context.Context, sourcedomain.KnowledgeObject, sourcedomain.KnowledgeDecision) error
	KnowledgeObjects(context.Context, string, string) ([]sourcedomain.KnowledgeObject, error)
	KnowledgeObject(context.Context, string, string, int) (sourcedomain.KnowledgeObject, error)
	CreateKnowledgePack(context.Context, sourcedomain.KnowledgePack) error
	KnowledgePacks(context.Context, string, string) ([]sourcedomain.KnowledgePack, error)
	KnowledgePack(context.Context, string, string) (sourcedomain.KnowledgePack, error)
	SaveKnowledgePack(context.Context, sourcedomain.KnowledgePack) error
	CreateKnowledgeSnapshot(context.Context, sourcedomain.KnowledgeSnapshot) error
	KnowledgeSnapshots(context.Context, string, string, string) ([]sourcedomain.KnowledgeSnapshot, error)
	KnowledgeSnapshot(context.Context, string, string) (sourcedomain.KnowledgeSnapshot, error)
	KnowledgeDecisions(context.Context, string, string) ([]sourcedomain.KnowledgeDecision, error)
}

type CatalogRepository interface {
	CreateEnvironment(context.Context, catalogdomain.Environment) error
	Environments(context.Context, string) ([]catalogdomain.Environment, error)
	Environment(context.Context, string, string) (catalogdomain.Environment, error)
	SaveEnvironment(context.Context, catalogdomain.Environment) error
	CreateSOP(context.Context, catalogdomain.SOPDefinition, catalogdomain.SOPVersion) error
	SaveSOPDefinition(context.Context, catalogdomain.SOPDefinition) error
	CreateSOPVersion(context.Context, catalogdomain.SOPVersion) error
	SOPs(context.Context, string) ([]catalogdomain.SOPSummary, error)
	SOP(context.Context, string, string) (catalogdomain.SOPSummary, error)
	SaveSOPVersion(context.Context, catalogdomain.SOPVersion) error
	PublishSOPVersion(context.Context, string, string, int, string, time.Time) (catalogdomain.SOPVersion, error)
	RetireSOPVersion(context.Context, string, string, int, time.Time) error
	SaveProjectSOPBinding(context.Context, catalogdomain.ProjectSOPBinding) error
	ProjectSOPBinding(context.Context, string, string) (catalogdomain.ProjectSOPBinding, error)
	ProjectSOPBindings(context.Context, string) ([]catalogdomain.ProjectSOPBinding, error)
}

type WorkRepository interface {
	CreateWorkTask(context.Context, work.WorkTask) error
	WorkTaskByIdempotencyKey(context.Context, string, string) (work.WorkTask, error)
	WorkTasks(context.Context, string, string) ([]work.WorkTask, error)
	WorkTask(context.Context, string, string) (work.WorkTask, error)
	SaveWorkTask(context.Context, work.WorkTask) error
	CreateInputItem(context.Context, work.InputItem) error
	InputItems(context.Context, string, string, string, string) ([]work.InputItem, error)
	InputItem(context.Context, string, string) (work.InputItem, error)
	InputItemByIdempotencyKey(context.Context, string, string) (work.InputItem, error)
	SaveInputItem(context.Context, work.InputItem, int) error
	CreateConversationImport(context.Context, work.ConversationImport) error
	ConversationImport(context.Context, string, string) (work.ConversationImport, error)
	ConversationImportByIdempotencyKey(context.Context, string, string) (work.ConversationImport, error)
	ConversationImportsForTask(context.Context, string, string) ([]work.ConversationImport, error)
	SaveConversationImport(context.Context, work.ConversationImport) error
	StageRuns(context.Context, string, string) ([]work.StageRun, error)
	CreateStageRun(context.Context, work.StageRun) error
	SaveStageRun(context.Context, work.StageRun) error
	CompleteStageRun(context.Context, work.StageRun, []work.TaskStageOutput) error
	TaskStageOutputs(context.Context, string, string) ([]work.TaskStageOutput, error)
}

type DeliveryRepository interface {
	CreateProviderProfile(context.Context, deliverydomain.ProviderProfile) error
	ProviderProfile(context.Context, string, string) (deliverydomain.ProviderProfile, error)
	SaveProviderBinding(context.Context, deliverydomain.ProviderBinding) error
	ProviderBinding(context.Context, string, string) (deliverydomain.ProviderBinding, error)
	CreateMediaGenerationJob(context.Context, deliverydomain.MediaGenerationJob) error
	PendingMediaGenerationJobs(context.Context, int) ([]deliverydomain.MediaGenerationJob, error)
	MediaGenerationJob(context.Context, string, string) (deliverydomain.MediaGenerationJob, error)
	MediaGenerationJobs(context.Context, string, string) ([]deliverydomain.MediaGenerationJob, error)
	SaveMediaGenerationJob(context.Context, deliverydomain.MediaGenerationJob, int) error
	CreateProviderAttempt(context.Context, deliverydomain.ProviderAttempt) error
	SaveProviderAttempt(context.Context, deliverydomain.ProviderAttempt) error
	ProviderAttempts(context.Context, string, string) ([]deliverydomain.ProviderAttempt, error)
	ProviderAttemptsByRuntimeJob(context.Context, string, string) ([]deliverydomain.ProviderAttempt, error)
	CreateMediaReview(context.Context, deliverydomain.MediaReview) error
	SaveMediaReview(context.Context, deliverydomain.MediaReview, int) error
	MediaReview(context.Context, string, string) (deliverydomain.MediaReview, error)
	MediaReviews(context.Context, string, string) ([]deliverydomain.MediaReview, error)
	CreateGateEvaluation(context.Context, reviewdomain.GateEvaluation) error
	GateEvaluations(context.Context, string, string) ([]reviewdomain.GateEvaluation, error)
	GateEvaluation(context.Context, string, string) (reviewdomain.GateEvaluation, error)
	SaveGateEvaluation(context.Context, reviewdomain.GateEvaluation) error
	CreateTaskRevision(context.Context, reviewdomain.TaskRevision) error
	TaskRevisions(context.Context, string, string) ([]reviewdomain.TaskRevision, error)
	TaskRevision(context.Context, string, string) (reviewdomain.TaskRevision, error)
	CreateTaskDelivery(context.Context, deliverydomain.TaskDelivery) error
	TaskDeliveries(context.Context, string, string) ([]deliverydomain.TaskDelivery, error)
	TaskDelivery(context.Context, string, string) (deliverydomain.TaskDelivery, error)
	SaveTaskDelivery(context.Context, deliverydomain.TaskDelivery) error
	CreateChannelBinding(context.Context, deliverydomain.ChannelBinding) error
	ChannelBindings(context.Context, string, string) ([]deliverydomain.ChannelBinding, error)
	ChannelBinding(context.Context, string, string) (deliverydomain.ChannelBinding, error)
	SaveChannelBinding(context.Context, deliverydomain.ChannelBinding) error
	CreateChannelPublication(context.Context, deliverydomain.ChannelPublication) error
	ChannelPublicationByIdempotencyKey(context.Context, string, string) (deliverydomain.ChannelPublication, error)
	ChannelPublications(context.Context, string, string) ([]deliverydomain.ChannelPublication, error)
	ChannelPublication(context.Context, string, string) (deliverydomain.ChannelPublication, error)
	SaveChannelPublication(context.Context, deliverydomain.ChannelPublication) error
	ApplyChannelCallback(context.Context, deliverydomain.ChannelPublication, deliverydomain.ChannelCallbackReceipt) (bool, error)
	CreateModelGenerationReceipt(context.Context, deliverydomain.ModelGenerationReceipt) error
	CreateModelGeneratedRevision(context.Context, reviewdomain.TaskRevision, deliverydomain.ModelGenerationReceipt) error
	ModelGenerationReceipts(context.Context, string, string) ([]deliverydomain.ModelGenerationReceipt, error)
}

type ContextRepository interface {
	CreateSnapshot(context.Context, sourcedomain.ContextSnapshot) error
	Snapshot(context.Context, string, string) (sourcedomain.ContextSnapshot, error)
}

type ReviewRepository interface {
	CreateApproval(context.Context, reviewdomain.ApprovalDecision) error
	Approvals(context.Context, string, string) ([]reviewdomain.ApprovalDecision, error)
	CreateReviewCycle(context.Context, reviewdomain.ReviewCycle) (reviewdomain.ReviewCycle, error)
	ReviewCycles(context.Context, string, string) ([]reviewdomain.ReviewCycle, error)
	SaveReviewCycle(context.Context, reviewdomain.ReviewCycle) error
	CreateReviewComment(context.Context, reviewdomain.ReviewComment) error
	ReviewComments(context.Context, string, string) ([]reviewdomain.ReviewComment, error)
	ReviewComment(context.Context, string, string) (reviewdomain.ReviewComment, error)
	SaveReviewComment(context.Context, reviewdomain.ReviewComment) error
	CreateReviewGrant(context.Context, reviewdomain.ReviewGrant) error
	ReviewGrant(context.Context, string, string) (reviewdomain.ReviewGrant, error)
	ReviewGrants(context.Context, string, string) ([]reviewdomain.ReviewGrant, error)
	ReviewGrantByTokenHash(context.Context, string) (reviewdomain.ReviewGrant, error)
	MarkReviewGrantVerified(context.Context, string, string, time.Time) error
	RevokeReviewGrant(context.Context, string, string, time.Time) error
	CreateSubmissionRevision(context.Context, reviewdomain.Submission, reviewdomain.SubmissionRevision, []reviewdomain.SourceDisclosure, reviewdomain.ReviewCycle) error
	SubmissionByWorkspaceType(context.Context, string, string, string, string) (reviewdomain.Submission, error)
	Submissions(context.Context, string, string) ([]reviewdomain.Submission, error)
	Submission(context.Context, string, string) (reviewdomain.Submission, error)
	SubmissionRevision(context.Context, string, string) (reviewdomain.SubmissionRevision, error)
	SubmissionRevisions(context.Context, string, string) ([]reviewdomain.SubmissionRevision, error)
	ApprovedSnapshots(context.Context, string, string, string) ([]reviewdomain.ApprovedSnapshot, error)
	ApprovedSnapshot(context.Context, string, string) (reviewdomain.ApprovedSnapshot, error)
	RecordSubmissionApproval(context.Context, reviewdomain.Submission, reviewdomain.ApprovalDecision) error
	ApproveSubmissionRevision(context.Context, reviewdomain.Submission, reviewdomain.ApprovedSnapshot, reviewdomain.ApprovalDecision) error
	RequestSubmissionChanges(context.Context, reviewdomain.Submission, reviewdomain.ApprovalDecision, reviewdomain.ReviewComment) error
	RejectSubmission(context.Context, reviewdomain.Submission, reviewdomain.ApprovalDecision, reviewdomain.ReviewComment) error
	CreateSubmissionReviewGrant(context.Context, reviewdomain.Submission, reviewdomain.ReviewGrant) error
	CompleteSubmissionClientReview(context.Context, reviewdomain.Submission, reviewdomain.ReviewGrant, reviewdomain.ApprovalDecision, *reviewdomain.ReviewComment, *reviewdomain.ApprovedSnapshot) error
}

type ArtifactRepository interface {
	CreateArtifact(context.Context, deliverydomain.Artifact) error
	ArtifactsByApprovedSnapshot(context.Context, string, string) ([]deliverydomain.Artifact, error)
	Artifact(context.Context, string, string) (deliverydomain.Artifact, error)
	CreateDeliveryPackage(context.Context, deliverydomain.DeliveryPackage, []deliverydomain.Artifact) error
	DeliveryPackages(context.Context, string, string) ([]deliverydomain.DeliveryPackage, error)
	DeliveryPackage(context.Context, string, string) (deliverydomain.DeliveryPackage, error)
}

type PerformanceRepository interface {
	CreatePerformanceImportBatch(context.Context, performancedomain.PerformanceImportBatch, []performancedomain.PerformanceObservation) error
	PerformanceImportBatches(context.Context, string, string) ([]performancedomain.PerformanceImportBatch, error)
	PerformanceImportBatch(context.Context, string, string) (performancedomain.PerformanceImportBatch, error)
	ExistingPerformanceDedupKeys(context.Context, string, string, []string) (map[string]string, error)
	PerformanceObservation(context.Context, string, string) (performancedomain.PerformanceObservation, error)
	PerformanceObservations(context.Context, string, string) ([]performancedomain.PerformanceObservation, error)
	CreateRatingDecision(context.Context, performancedomain.RatingDecision) error
	RatingDecisions(context.Context, string, string) ([]performancedomain.RatingDecision, error)
}

type AuditRepository interface {
	AppendAudit(context.Context, auditdomain.AuditEvent) error
	AuditEvents(context.Context, string, string, int) ([]auditdomain.AuditEvent, error)
}
