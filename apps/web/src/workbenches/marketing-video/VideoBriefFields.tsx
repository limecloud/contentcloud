import type { BusinessBriefFormState } from '../../workbench/contract/businessBrief';

interface VideoBriefFieldsProps {
  value:BusinessBriefFormState;
  onChange:(next:BusinessBriefFormState)=>void;
}

export function VideoBriefFields({value,onChange}:VideoBriefFieldsProps){
  const update=(key:keyof BusinessBriefFormState)=>(event:React.ChangeEvent<HTMLInputElement>)=>onChange({...value,[key]:event.target.value});
  return <section className="studio-business-brief"><header><span>视频业务简报</span><small>受众、渠道和表达边界会随任务固定，供剧本、分镜和成片流程引用。</small></header><label className="studio-field"><span>目标受众</span><input value={value.audience} onChange={update('audience')} placeholder="例如：第一次认识品牌的年轻家庭"/></label><label className="studio-field"><span>发布渠道</span><input value={value.channel} onChange={update('channel')} placeholder="例如：抖音竖屏短视频"/></label><label className="studio-field"><span>表达语气</span><input value={value.tone} onChange={update('tone')} placeholder="例如：真实、克制、有生活感"/></label><label className="studio-field"><span>必须出现的关键词</span><input value={value.keywords} onChange={update('keywords')} placeholder="用逗号分隔，例如：主理人、产品、使用场景"/></label></section>;
}
