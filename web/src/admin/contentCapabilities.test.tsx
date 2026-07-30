import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { PlatformTenant } from '../types';
import { AdminFeedback } from './AdminShell';
import { TenantTable } from './components';

describe('tenant content capability controls',()=>{
  it('shows video as the fixed default and WeChat disabled by default',()=>{
    const markup=table(['video_script'],'');
    expect(markup).toContain('视频剧本为默认能力');
    expect(markup).toContain('公众号');
    expect(markup).not.toMatch(/type="checkbox"[^>]*checked/);
  });

  it('shows an enabled WeChat capability from the server projection',()=>{
    const markup=table(['video_script','wechat_article'],'');
    expect(markup).toMatch(/type="checkbox"[^>]*checked/);
  });

  it('disables the switch and exposes progress while an update is busy',()=>{
    const markup=table(['video_script'],'tenant-1:wechat_article');
    expect(markup).toMatch(/type="checkbox"[^>]*disabled/);
    expect(markup).toContain('更新中');
  });

  it('renders the API error returned through the admin context',()=>{
    const markup=renderToStaticMarkup(<AdminFeedback error="当前租户不存在" onClose={()=>{}}/>);
    expect(markup).toContain('banner-error');
    expect(markup).toContain('当前租户不存在');
  });
});

function table(contentTypes:PlatformTenant['content_types'],busy:string) {
  const tenant:PlatformTenant={id:'tenant-1',name:'示例租户',slug:'example',status:'active',created_at:'2026-07-29T00:00:00Z',member_count:1,project_count:2,device_count:0,active_run_count:0,content_types:contentTypes};
  return renderToStaticMarkup(<TenantTable tenants={[tenant]} currentTenantID="" busy={busy} onContentAction={()=>{}}/>);
}
