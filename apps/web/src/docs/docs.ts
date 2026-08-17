import { api } from '../api';
import type { AgentCapabilityID, AgentSupportStatus } from '../agentHandoff';

export type DocsStatus = 'available' | 'limited' | 'planned';

export interface DocsPageSummary {
  slug: string;
  title: string;
  description: string;
  kind: string;
  status: DocsStatus;
}

export interface DocsPage extends DocsPageSummary { markdown: string }
export interface DocsSection { id:string; title:string; description:string; pages:DocsPageSummary[] }
export interface DocsClient {
  id:string;
  display_name:string;
  status:DocsStatus;
  summary:string;
  page_slug:string;
  capabilities:Array<{id:AgentCapabilityID;status:AgentSupportStatus}>;
}
export interface DocsContentType { id:string; title:string; status:DocsStatus; summary:string; page_slug:string }
export interface DocsGuide { id:string; title:string; client_id:string; content_type_id:string; status:DocsStatus; summary:string; page_slug:string }
export interface DocsCatalog {
  schema_version:'contentcloud.docs-catalog/1.0';
  home:DocsPageSummary;
  pages:DocsPageSummary[];
  sections:DocsSection[];
  clients:DocsClient[];
  content_types:DocsContentType[];
  guides:DocsGuide[];
}

const slugPattern=/^[a-z0-9][a-z0-9-]*(\/[a-z0-9][a-z0-9-]*)*$/;
const statuses=new Set<DocsStatus>(['available','limited','planned']);

export async function loadDocsCatalog():Promise<DocsCatalog> {
  return validateDocsCatalog(await api<unknown>('/api/docs/catalog'));
}

export async function loadDocsPage(slug:string):Promise<DocsPage> {
  if(!slugPattern.test(slug))throw new Error('文档地址无效');
  const value=await api<unknown>(`/api/docs/pages/${slug.split('/').map(encodeURIComponent).join('/')}`);
  return validateDocsPage(value,slug);
}

export function validateDocsCatalog(value:unknown):DocsCatalog {
  if(!isRecord(value)||value.schema_version!=='contentcloud.docs-catalog/1.0'||!isPageSummary(value.home)||!Array.isArray(value.pages)||!Array.isArray(value.sections)||!Array.isArray(value.clients)||!Array.isArray(value.content_types)||!Array.isArray(value.guides))throw new Error('文档目录结构不受支持');
  if(!value.pages.every(isPageSummary)||!value.sections.every(isSection)||!value.clients.every(isClient)||!value.content_types.every(isContentType)||!value.guides.every(isGuide))throw new Error('文档目录条目无效');
  const pageSlugs=value.pages.map(page=>page.slug);
  if(new Set(pageSlugs).size!==pageSlugs.length||value.clients.length===0||value.content_types.length===0)throw new Error('文档目录不完整');
  return value as unknown as DocsCatalog;
}

export function validateDocsPage(value:unknown,expectedSlug:string):DocsPage {
  if(!isPageSummary(value)||value.slug!==expectedSlug||!('markdown' in value)||typeof value.markdown!=='string'||value.markdown.length===0)throw new Error('文档页面结构不受支持');
  return value as unknown as DocsPage;
}

export function docsWebPath(slug:string):string|undefined {
  if(!slugPattern.test(slug))return undefined;
  if(slug==='overview')return '/docs';
  const parts=slug.split('/');
  if(parts[0]==='clients'&&parts.length===2)return `/docs/clients/${parts[1]}`;
  if(parts[0]==='content-types'&&parts.length===2)return `/docs/content/${parts[1]}`;
  if(parts[0]==='guides'&&parts.length===3)return `/docs/guides/${parts[1]}/${parts[2]}`;
  return `/docs/pages/${slug}`;
}

export function resolveMarkdownHref(href:string,currentSlug:string):{href:string;external:boolean}|undefined {
  const value=href.trim();
  if(!value)return undefined;
  if(value.startsWith('#'))return {href:value,external:false};
  if(value.startsWith('/')&&!value.startsWith('//'))return {href:value,external:false};
  try {
    const absolute=new URL(value);
    if(absolute.protocol==='http:'||absolute.protocol==='https:')return {href:absolute.toString(),external:true};
    return undefined;
  } catch {/* Relative Markdown link. */}

  let resolved:URL;
  try {resolved=new URL(value,`https://contentcloud.docs/source/${currentSlug}`)} catch {return undefined}
  if(resolved.origin!=='https://contentcloud.docs'||!resolved.pathname.startsWith('/source/'))return undefined;
  let slug=resolved.pathname.slice('/source/'.length);
  if(slug.endsWith('.md'))slug=slug.slice(0,-3);
  const path=docsWebPath(slug);
  return path?{href:path+resolved.search+resolved.hash,external:false}:undefined;
}

export function docsStatusLabel(status:DocsStatus):string {
  return status==='available'?'可用':status==='limited'?'有限支持':'即将支持';
}

function isPageSummary(value:unknown):value is DocsPageSummary {
  return isRecord(value)&&typeof value.slug==='string'&&slugPattern.test(value.slug)&&typeof value.title==='string'&&Boolean(value.title)&&typeof value.description==='string'&&Boolean(value.description)&&typeof value.kind==='string'&&statuses.has(value.status);
}
function isSection(value:unknown):value is DocsSection {return isRecord(value)&&typeof value.id==='string'&&typeof value.title==='string'&&typeof value.description==='string'&&Array.isArray(value.pages)&&value.pages.every(isPageSummary)}
function isClient(value:unknown):value is DocsClient {return isRecord(value)&&typeof value.id==='string'&&typeof value.display_name==='string'&&statuses.has(value.status)&&typeof value.summary==='string'&&typeof value.page_slug==='string'&&slugPattern.test(value.page_slug)&&Array.isArray(value.capabilities)&&value.capabilities.every(capability=>isRecord(capability)&&typeof capability.id==='string'&&(capability.status==='available'||capability.status==='planned'))}
function isContentType(value:unknown):value is DocsContentType {return isRecord(value)&&typeof value.id==='string'&&typeof value.title==='string'&&statuses.has(value.status)&&typeof value.summary==='string'&&typeof value.page_slug==='string'&&slugPattern.test(value.page_slug)}
function isGuide(value:unknown):value is DocsGuide {return isRecord(value)&&typeof value.id==='string'&&typeof value.title==='string'&&typeof value.client_id==='string'&&typeof value.content_type_id==='string'&&statuses.has(value.status)&&typeof value.summary==='string'&&typeof value.page_slug==='string'&&slugPattern.test(value.page_slug)}
function isRecord(value:unknown):value is Record<string,any> {return typeof value==='object'&&value!==null&&!Array.isArray(value)}
