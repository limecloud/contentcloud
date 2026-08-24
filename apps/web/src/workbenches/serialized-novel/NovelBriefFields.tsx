import type { BusinessBriefFormState } from '../../workbench/contract/businessBrief';

interface NovelBriefFieldsProps {
  value:BusinessBriefFormState;
  onChange:(next:BusinessBriefFormState)=>void;
}

export function NovelBriefFields({value,onChange}:NovelBriefFieldsProps){
  const update=(key:keyof BusinessBriefFormState)=>(event:React.ChangeEvent<HTMLInputElement>)=>onChange({...value,[key]:event.target.value});
  return <section className="studio-business-brief"><header><span>小说业务简报</span><small>读者、连载平台、文风和世界观关键词会随任务固定，供 Canon、卷章规划和章节流程引用。</small></header><label className="studio-field"><span>目标读者</span><input value={value.audience} onChange={update('audience')} placeholder="例如：喜欢东方奇幻长线成长的读者"/></label><label className="studio-field"><span>连载平台</span><input value={value.channel} onChange={update('channel')} placeholder="例如：Web Novel"/></label><label className="studio-field"><span>文风边界</span><input value={value.tone} onChange={update('tone')} placeholder="例如：克制、悬疑、少量幽默"/></label><label className="studio-field"><span>世界观关键词</span><input value={value.keywords} onChange={update('keywords')} placeholder="用逗号分隔，例如：城邦、契约、失踪者"/></label></section>;
}
