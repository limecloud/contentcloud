import type { BusinessBriefFormState } from '../../workbench/contract/businessBrief';

interface ArticleBriefFieldsProps {
  value:BusinessBriefFormState;
  onChange:(next:BusinessBriefFormState)=>void;
}

export function ArticleBriefFields({value,onChange}:ArticleBriefFieldsProps){
  const update=(key:keyof BusinessBriefFormState)=>(event:React.ChangeEvent<HTMLInputElement>)=>onChange({...value,[key]:event.target.value});
  return <section className="studio-business-brief"><header><span>文章业务简报</span><small>这些字段会随任务固定，供文章工作台和后续流程使用。</small></header><label className="studio-field"><span>目标受众</span><input value={value.audience} onChange={update('audience')} placeholder="例如：准备第一次购买的年轻妈妈"/></label><label className="studio-field"><span>发布渠道</span><input value={value.channel} onChange={update('channel')} placeholder="例如：微信公众号"/></label><label className="studio-field"><span>表达语气</span><input value={value.tone} onChange={update('tone')} placeholder="例如：可信、清楚、不夸张"/></label><label className="studio-field"><span>关键词</span><input value={value.keywords} onChange={update('keywords')} placeholder="用逗号分隔，例如：成分、使用方法、注意事项"/></label></section>;
}
