import { BriefcaseBusiness } from 'lucide-react';
import { ALL_EXPERIENCES } from './studioScope';
import type { StudioExperience } from './studioTypes';

const typeLabels:Record<string,string>={marketing_video:'营销视频',video_script:'视频脚本',article:'文章',wechat_article:'公众号文章',article_content:'文章',commerce:'电商内容',ecommerce:'电商内容',douyin_commerce:'抖音电商',product_content:'商品内容',serialized_novel:'连载小说'};

export function StudioScopeBanner({experience,experienceID,projectCount}:{experience?:StudioExperience;experienceID:string;projectCount:number}){
  const isAll=experienceID===ALL_EXPERIENCES||!experienceID;
  const type=experience?typeLabels[experience.content_type.trim().toLowerCase()]||experience.content_type:'未选择';
  return <div className="studio-scope-banner"><span><BriefcaseBusiness size={15}/>业务范围</span><strong>{experience?.name||(isAll?'全部业务':'未开通业务')}</strong><small>{isAll?`${projectCount} 个可用项目 · 当前显示全部业务`:experience?`${type} · ${projectCount} 个可用项目`:'请从顶部业务菜单选择一个工作台'}</small></div>;
}
