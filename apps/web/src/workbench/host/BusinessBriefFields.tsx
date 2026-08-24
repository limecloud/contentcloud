import { ArticleBriefFields } from '../../workbenches/article/ArticleBriefFields';
import { CommerceBriefFields } from '../../workbenches/commerce/CommerceBriefFields';
import { VideoBriefFields } from '../../workbenches/marketing-video/VideoBriefFields';
import { NovelBriefFields } from '../../workbenches/serialized-novel/NovelBriefFields';
import { businessContentType, type BusinessBriefFormState } from '../contract/businessBrief';

interface BusinessBriefFieldsProps {
  contentType:string;
  value:BusinessBriefFormState;
  onChange:(next:BusinessBriefFormState)=>void;
}

export function BusinessBriefFields({contentType,value,onChange}:BusinessBriefFieldsProps){
  switch(businessContentType(contentType)){
    case 'marketing_video':
    case 'video_script':
      return <VideoBriefFields value={value} onChange={onChange}/>;
    case 'article':
    case 'wechat_article':
    case 'article_content':
      return <ArticleBriefFields value={value} onChange={onChange}/>;
    case 'serialized_novel':
      return <NovelBriefFields value={value} onChange={onChange}/>;
    case 'commerce':
    case 'ecommerce':
    case 'douyin_commerce':
    case 'product_content':
      return <CommerceBriefFields value={value} onChange={onChange}/>;
    default:
      return null;
  }
}
