import type { WorkbenchDefinition } from '../../workbench/registry/registry';
import { navigation } from '../../workbench/contract/navigation';

export const articleWorkbench:WorkbenchDefinition={
  key:'article',contentTypes:['article','wechat_article','article_content'],eyebrow:'文章创作工作台',sourceTitle:'选择资料与引用',sourceDetail:'从来源、知识快照或已确认文章开始组织本次写作',
  navigation:[navigation('overview','文章首页','pen-line'),navigation('tasks','文章任务','archive'),navigation('library','资料与引用','book-open'),navigation('deliveries','文章交付','file-check')],
  stepTitles:['选题','资料与引用','文章草稿','校对与确认','交付'],
  panels:[
    {id:'brief',title:'选题与资料',detail:'明确文章受众和主张，整理可以追溯的资料与引用。',tone:'source',icon:'book-open',stepIDs:['brief','knowledge','sources'],stepTitles:['选题','资料与引用'],target:'start',actionLabel:'开始选题'},
    {id:'writing',title:'文章写作',detail:'在可回溯的资料基础上完成草稿、标题和正文结构。',tone:'knowledge',icon:'pen-line',stepIDs:['draft','writing'],stepTitles:['文章草稿'],target:'tasks',actionLabel:'查看文章任务'},
    {id:'proofing',title:'校对与交付',detail:'检查引用、品牌表达和发布格式，准备最终文章交付。',tone:'review',icon:'file-check',stepIDs:['quality','delivery','proofread'],stepTitles:['校对与确认','交付'],target:'deliveries',actionLabel:'查看文章交付'},
    {id:'assets',title:'资料与引用',detail:'管理文章资料和已经确认的内容版本。',tone:'knowledge',icon:'archive',stepIDs:[],stepTitles:['资料库','创作结果'],target:'assets',actionLabel:'打开资料'},
  ],
};
