import type { BusinessBriefFormState } from '../../workbench/contract/businessBrief';

interface CommerceBriefFieldsProps {
  value:BusinessBriefFormState;
  onChange:(next:BusinessBriefFormState)=>void;
}

export function CommerceBriefFields({value,onChange}:CommerceBriefFieldsProps){
  const update=(key:keyof BusinessBriefFormState)=>(event:React.ChangeEvent<HTMLInputElement|HTMLTextAreaElement>)=>onChange({...value,[key]:event.target.value});
  return <section className="studio-business-brief"><header><span>商品业务简报</span><small>商品事实和购买理由会作为任务输入快照，不会被工作台另行复制。</small></header><label className="studio-field"><span>渠道</span><input value={value.channel} onChange={update('channel')} placeholder="例如：抖音短视频"/></label><label className="studio-field"><span>目标人群</span><input value={value.targetAudience} onChange={update('targetAudience')} placeholder="例如：关注低糖零食的上班族"/></label><label className="studio-field"><span>商品事实</span><textarea value={value.productFacts} onChange={update('productFacts')} rows={4} placeholder="每行一条，格式为 属性: 事实，例如：净含量: 500g"/></label><label className="studio-field"><span>购买理由</span><textarea value={value.offerPoints} onChange={update('offerPoints')} rows={3} placeholder="用逗号或换行分隔，例如：低糖、独立包装、办公室方便携带"/></label></section>;
}
