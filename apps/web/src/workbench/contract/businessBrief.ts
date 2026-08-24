export interface BusinessBriefFormState {
  audience:string;
  channel:string;
  tone:string;
  keywords:string;
  targetAudience:string;
  productFacts:string;
  offerPoints:string;
}

export interface BusinessBriefContract {
  schema_version:string;
  audience?:string;
  channel?:string;
  tone?:string;
  keywords?:string[];
  product_facts?:Record<string,string>;
  offer_points?:string[];
  target_audience?:string;
}

export function emptyBusinessBriefForm():BusinessBriefFormState {
  return {audience:'',channel:'',tone:'',keywords:'',targetAudience:'',productFacts:'',offerPoints:''};
}

export function businessContentType(value:string):string {
  return value.trim().toLowerCase().replace(/-/g,'_');
}

export function parseBriefList(value:string):string[] {
  return value.split(/\n|,/).map(item=>item.trim()).filter(Boolean);
}

function parseProductFacts(value:string):Record<string,string> {
  const facts:Record<string,string>={};
  value.split('\n').forEach(line=>{
    const separator=line.indexOf(':');
    if(separator<=0)return;
    const key=line.slice(0,separator).trim();
    const fact=line.slice(separator+1).trim();
    if(key&&fact)facts[key]=fact;
  });
  return facts;
}

export function buildBusinessBrief(contentType:string,value:BusinessBriefFormState):BusinessBriefContract|undefined {
  const type=businessContentType(contentType);
  if(['marketing_video','video_script','article','wechat_article','article_content'].includes(type)){
    return {schema_version:'contentcloud.business-brief/1.0',audience:value.audience.trim(),channel:value.channel.trim(),tone:value.tone.trim(),keywords:parseBriefList(value.keywords)};
  }
  if(['commerce','ecommerce','douyin_commerce','product_content'].includes(type)){
    return {schema_version:'contentcloud.business-brief/1.0',channel:value.channel.trim(),target_audience:value.targetAudience.trim(),product_facts:parseProductFacts(value.productFacts),offer_points:parseBriefList(value.offerPoints)};
  }
  return undefined;
}

export function businessBriefReady(contentType:string,value:BusinessBriefFormState):boolean {
  const type=businessContentType(contentType);
  if(['marketing_video','video_script','article','wechat_article','article_content'].includes(type)){
    return Boolean(value.audience.trim()&&value.channel.trim()&&value.tone.trim()&&parseBriefList(value.keywords).length);
  }
  if(['commerce','ecommerce','douyin_commerce','product_content'].includes(type)){
    return Boolean(value.channel.trim()&&value.targetAudience.trim()&&Object.keys(parseProductFacts(value.productFacts)).length&&parseBriefList(value.offerPoints).length);
  }
  return true;
}
