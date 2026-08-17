import { describe, expect, it } from 'vitest';
import { matchRoutes } from 'react-router-dom';
import { appRoutes } from '../router';
import { docsWebPath, resolveMarkdownHref, validateDocsCatalog, validateDocsPage } from './docs';

describe('documentation routes and contracts',()=>{
  it('maps catalog slugs to stable public routes',()=>{
    expect(docsWebPath('overview')).toBe('/docs');
    expect(docsWebPath('clients/codex')).toBe('/docs/clients/codex');
    expect(docsWebPath('content-types/marketing-video')).toBe('/docs/content/marketing-video');
    expect(docsWebPath('guides/marketing-video/codex')).toBe('/docs/guides/marketing-video/codex');
    expect(docsWebPath('../internal')).toBeUndefined();
    expect(matchRoutes(appRoutes,'/docs/clients/openclaw')?.map(match=>match.route.path)).toEqual(['/docs','clients/:clientID']);
    expect(matchRoutes(appRoutes,'/docs/pages/troubleshooting/workspace-and-handoff')?.map(match=>match.route.path)).toEqual(['/docs','pages/*']);
  });

  it('resolves relative Markdown links without allowing unsafe protocols',()=>{
    expect(resolveMarkdownHref('../content-types/marketing-video.md','clients/codex')).toEqual({href:'/docs/content/marketing-video',external:false});
    expect(resolveMarkdownHref('../../troubleshooting/workspace-and-handoff.md','guides/marketing-video/codex')).toEqual({href:'/docs/pages/troubleshooting/workspace-and-handoff',external:false});
    expect(resolveMarkdownHref('/codex','clients/codex')).toEqual({href:'/codex',external:false});
    expect(resolveMarkdownHref('https://example.com/guide','clients/codex')?.external).toBe(true);
    expect(resolveMarkdownHref('javascript:alert(1)','clients/codex')).toBeUndefined();
  });

  it('validates the public API shape and exact page target',()=>{
    const page={slug:'clients/codex',title:'Codex',description:'guide',kind:'client',status:'available',markdown:'# Codex'};
    expect(validateDocsPage(page,'clients/codex').markdown).toBe('# Codex');
    expect(()=>validateDocsPage(page,'clients/cursor')).toThrow();
    const catalog={schema_version:'contentcloud.docs-catalog/1.0',home:{...page,slug:'overview'},pages:[page],sections:[],clients:[{id:'codex',display_name:'Codex',status:'available',summary:'ready',page_slug:'clients/codex',capabilities:[]}],content_types:[{id:'marketing-video',title:'营销视频',status:'available',summary:'ready',page_slug:'content-types/marketing-video'}],guides:[]};
    expect(validateDocsCatalog(catalog).clients[0].id).toBe('codex');
    expect(()=>validateDocsCatalog({...catalog,schema_version:'contentcloud.docs-catalog/2.0'})).toThrow();
  });
});
