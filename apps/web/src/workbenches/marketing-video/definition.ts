import type { WorkbenchDefinition } from '../../workbench/registry/registry';
import { navigation } from '../../workbench/contract/navigation';

export const marketingVideoWorkbench:WorkbenchDefinition={
  key:'marketing-video',contentTypes:['marketing_video','marketing-video'],eyebrow:'视频生产工作台',sourceTitle:'选择素材与已有内容',sourceDetail:'从品牌资料、项目参考或已确认成果开始',
  navigation:[navigation('overview','视频首页','video'),navigation('tasks','视频任务','archive'),navigation('assets','视频素材','image'),navigation('deliveries','视频交付','package-check')],
  stepTitles:['灵感采集','人物原型','营销剧本','视频分镜','候选成片','交付准备'],
  panels:[
    {id:'direction',title:'灵感与人物',detail:'收集可信参考，确定人物定位、受众和表达方向。',tone:'source',icon:'lightbulb',stepIDs:['inspiration','persona'],stepTitles:['灵感采集','人物原型'],target:'start',actionLabel:'开始策划'},
    {id:'production',title:'剧本与分镜',detail:'确认营销剧本版本，锁定镜头、画面、素材和连续性。',tone:'strategy',icon:'file-text',stepIDs:['script','storyboard'],stepTitles:['营销剧本','视频分镜'],target:'tasks',actionLabel:'选择创作任务'},
    {id:'delivery',title:'成片与交付',detail:'选择候选成片，完成最终确认并下载固定交付包。',tone:'production',icon:'video',stepIDs:['media','delivery'],stepTitles:['候选成片','交付准备'],target:'deliveries',actionLabel:'查看成片与交付'},
    {id:'assets',title:'已有内容',detail:'整理资料，也可以从已确认的创作结果开始下一次创作。',tone:'knowledge',icon:'archive',stepIDs:[],stepTitles:['我的资料','创作结果'],target:'assets',actionLabel:'打开资料'},
  ],
};
