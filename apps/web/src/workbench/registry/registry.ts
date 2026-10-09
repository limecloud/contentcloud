import { entityDisplayName } from "../../entity-display-name";
import { articleWorkbench } from '../../workbenches/article/definition';
import { commerceWorkbench } from '../../workbenches/commerce/definition';
import { marketingVideoWorkbench } from '../../workbenches/marketing-video/definition';
import { serializedNovelWorkbench } from '../../workbenches/serialized-novel/definition';
import { routeByNavigationID } from '../contract/navigation';

export type WorkbenchTone='source'|'knowledge'|'strategy'|'production'|'review';
export type WorkbenchPanelTarget='start'|'tasks'|'assets'|'deliveries';
export type WorkbenchIconName='archive'|'book-open'|'file-check'|'file-text'|'folder'|'image'|'lightbulb'|'list-checks'|'package-check'|'pen-line'|'shopping-bag'|'tags'|'video'|'scroll-text';

const approvedIcons=new Set<WorkbenchIconName>(['archive','book-open','file-check','file-text','folder','image','lightbulb','list-checks','package-check','pen-line','shopping-bag','tags','video','scroll-text']);

export interface WorkbenchNavigationItem {
  id:string;
  label:string;
  icon:WorkbenchIconName;
  href:string;
}

export interface WorkbenchPanelDefinition {
  id:string;
  title:string;
  detail:string;
  tone:WorkbenchTone;
  icon:WorkbenchIconName;
  stepIDs:string[];
  stepTitles:string[];
  target:WorkbenchPanelTarget;
  actionLabel:string;
}

export interface WorkbenchDefinition {
  key:string;
  contentTypes:string[];
  eyebrow:string;
  sourceTitle:string;
  sourceDetail:string;
  navigation:WorkbenchNavigationItem[];
  stepTitles:string[];
  panels:WorkbenchPanelDefinition[];
}

export interface WorkbenchExperienceInput {
  content_type:string;
  step_titles:string[];
  workbench?:{
    plugin_id:string;
    version:string;
    digest:string;
    layout:string;
    density:string;
    theme:string;
    navigation:{id:string;label:string;icon:string}[];
    stages:{id:string;label:string;outcome:string;primary_action:string}[];
    panels?:{id:string;title:string;detail:string;tone:string;icon:string;stage_ids:string[];target:string;action_label:string}[];
  };
}

const fallback:WorkbenchDefinition={
  key:'general',contentTypes:[],eyebrow:'内容工作台',sourceTitle:'选择资料与已有内容',sourceDetail:'从项目资料或已确认成果开始新的内容任务',stepTitles:['需求','资料','内容','确认','交付'],
  navigation:[{id:'overview',label:'工作台首页',icon:'archive',href:routeByNavigationID.overview},{id:'tasks',label:'我的任务',icon:'archive',href:routeByNavigationID.tasks},{id:'assets',label:'我的资料',icon:'archive',href:routeByNavigationID.assets},{id:'deliveries',label:'交付',icon:'package-check',href:routeByNavigationID.deliveries}],
  panels:[
    {id:'brief',title:'需求与资料',detail:'明确这次要完成的内容，选择可引用的项目资料。',tone:'source',icon:'folder',stepIDs:['brief','sources'],stepTitles:['需求','资料'],target:'start',actionLabel:'开始创作'},
    {id:'content',title:'内容工作面',detail:'在当前项目中继续编辑、检查并保存内容版本。',tone:'strategy',icon:'file-text',stepIDs:['content','draft'],stepTitles:['内容'],target:'tasks',actionLabel:'查看任务'},
    {id:'delivery',title:'确认与交付',detail:'核对结果版本和交付文件，完成可追溯的交接。',tone:'review',icon:'package-check',stepIDs:['review','delivery'],stepTitles:['确认','交付'],target:'deliveries',actionLabel:'查看交付'},
    {id:'assets',title:'已有内容',detail:'管理资料和已经确认的结果版本。',tone:'knowledge',icon:'archive',stepIDs:[],stepTitles:['资料','结果'],target:'assets',actionLabel:'打开资料'},
  ],
};

const definitions=[marketingVideoWorkbench,articleWorkbench,commerceWorkbench,serializedNovelWorkbench];
const firstPartyPluginIDs=new Set(['contentcloud-workbench-marketing-video','contentcloud-workbench-article','contentcloud-workbench-commerce','contentcloud-workbench-serialized-novel']);

function normalize(value:string):string {
  return value.trim().toLowerCase().replace(/\s+/g,'_');
}

function approvedIcon(value:string):WorkbenchIconName {
  return approvedIcons.has(value as WorkbenchIconName)?value as WorkbenchIconName:'archive';
}

export function getWorkbenchDefinition(experience?:WorkbenchExperienceInput):WorkbenchDefinition {
  const contentType=normalize(experience?.content_type||'');
  const base=definitions.find(item=>item.contentTypes.includes(contentType))||fallback;
  const serverStages=experience?.workbench?.stages||[];
  const serverStageTitles=serverStages.map(stage=>stage.label).filter(Boolean);
  const serverTitles=experience?.step_titles?.filter(Boolean)||[];
  const serverDriven=Boolean(experience?.workbench?.plugin_id&&!firstPartyPluginIDs.has(experience.workbench.plugin_id));
  const stepTitles=serverDriven
    ? (serverStageTitles.length?serverStageTitles:serverTitles)
    : (serverTitles.length?serverTitles:serverStageTitles);
  const serverNavigation=experience?.workbench?.navigation||[];
  const serverPanels=experience?.workbench?.panels||[];
  const resolvedNavigation=serverNavigation.length
    ? serverNavigation.map(item=>({id:item.id,label:item.label,icon:approvedIcon(item.icon),href:routeByNavigationID[item.id]||'/studio'}))
    : base.navigation;
  return {
    ...base,
    navigation:resolvedNavigation,
    stepTitles:stepTitles.length?stepTitles:base.stepTitles,
    panels:(serverPanels.length?serverPanels.map(panel=>({id:panel.id,title:panel.title,detail:panel.detail,tone:(panel.tone as WorkbenchTone),icon:approvedIcon(panel.icon),stepIDs:panel.stage_ids,stepTitles:panel.stage_ids.map(stageID=>entityDisplayName(serverStages.find(stage=>stage.id===stageID)?.label,"未命名阶段",stageID)),target:(panel.target as WorkbenchPanelTarget),actionLabel:panel.action_label})):base.panels).map(panel=>({...panel,stepTitles:serverPanels.length?panel.stepTitles:panel.stepTitles.map((title,index)=>stepTitles[base.stepTitles.indexOf(title)]||stepTitles[index]||title)})),
  };
}
