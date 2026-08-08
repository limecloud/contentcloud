import { api, post, upload } from '../../api';
import type { EvidenceSpan, KnowledgeObject, KnowledgePack, KnowledgeQueryResult, KnowledgeSnapshot, Source, SourceRevision } from '../../types';

export type GovernedKnowledgeObject = KnowledgeObject & {
  allowed_actions?:string[];
  governance_state?:string;
  governance_message?:string;
};

const studioPath=(suffix:string)=>`/api/studio${suffix}`;

export const knowledgeApi={
  objects:(projectID:string)=>api<GovernedKnowledgeObject[]>(studioPath(`/projects/${encodeURIComponent(projectID)}/knowledge-objects`)),
  transition:(objectID:string,input:{expected_version:number;expected_digest:string;decision:'approve'|'reject';reason:string})=>post<{object:KnowledgeObject}>(studioPath(`/knowledge-objects/${encodeURIComponent(objectID)}/transitions`),input),
  packs:(projectID:string)=>api<KnowledgePack[]>(studioPath(`/projects/${encodeURIComponent(projectID)}/knowledge-packs`)),
  createPack:(projectID:string,input:unknown)=>post<KnowledgePack>(studioPath(`/projects/${encodeURIComponent(projectID)}/knowledge-packs`),input),
  publishPack:(packID:string)=>post(studioPath(`/knowledge-packs/${encodeURIComponent(packID)}/publish`)),
  snapshots:(projectID:string,packID:string)=>api<KnowledgeSnapshot[]>(studioPath(`/projects/${encodeURIComponent(projectID)}/knowledge-packs/${encodeURIComponent(packID)}/snapshots`)),
  query:(input:unknown)=>post<KnowledgeQueryResult>(studioPath('/knowledge/query'),input),
  sources:(projectID:string)=>api<Source[]>(studioPath(`/projects/${encodeURIComponent(projectID)}/sources`)),
  sourceRevisions:(sourceID:string)=>api<SourceRevision[]>(studioPath(`/sources/${encodeURIComponent(sourceID)}/revisions`)),
  evidence:(revisionID:string)=>api<EvidenceSpan[]>(studioPath(`/source-revisions/${encodeURIComponent(revisionID)}/evidence`)),
  uploadSource:(projectID:string,form:FormData)=>upload<SourceRevision>(studioPath(`/projects/${encodeURIComponent(projectID)}/sources/upload`),form),
  uploadSourceRevision:(sourceID:string,form:FormData)=>upload<SourceRevision>(studioPath(`/sources/${encodeURIComponent(sourceID)}/revisions/upload`),form),
};
