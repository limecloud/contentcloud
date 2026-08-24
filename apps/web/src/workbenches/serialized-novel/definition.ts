import type { WorkbenchDefinition } from '../../workbench/registry/registry';
import { navigation } from '../../workbench/contract/navigation';

export const serializedNovelWorkbench:WorkbenchDefinition={
  key:'serialized-novel',contentTypes:['serialized_novel'],eyebrow:'连载小说工作台',sourceTitle:'选择 Canon 与资料',sourceDetail:'从已固定的世界观、卷章规划和资料开始推进章节生产',
  navigation:[navigation('overview','小说首页','book-open'),navigation('tasks','章节任务','list-checks'),navigation('library','Canon 与资料','archive'),navigation('deliveries','章节交付','file-check')],
  stepTitles:['世界观 Canon','卷章规划','章节候选','连续性校验','编辑与确认','章节交付'],
  panels:[
    {id:'canon',title:'Canon 与卷章',detail:'固定角色、世界规则、时间线和本卷章节目标，所有候选都从这里引用。',tone:'knowledge',icon:'book-open',stepIDs:['canon','outline'],stepTitles:['世界观 Canon','卷章规划'],target:'start',actionLabel:'开始规划章节'},
    {id:'chapter',title:'章节写作',detail:'生成并编辑章节候选，保留角色、地点和伏笔引用以便连续性校验。',tone:'strategy',icon:'pen-line',stepIDs:['chapter'],stepTitles:['章节候选'],target:'tasks',actionLabel:'查看章节任务'},
    {id:'review',title:'连续性与编辑',detail:'先通过确定性连续性检查，再进入编辑和客户确认，批准后才可交付。',tone:'review',icon:'file-check',stepIDs:['continuity','edit'],stepTitles:['连续性校验','编辑与确认'],target:'tasks',actionLabel:'查看确认事项'},
    {id:'delivery',title:'章节交付',detail:'从 ApprovedSnapshot 生成 JSON、Markdown 和表格交付文件，保留章节血缘。',tone:'production',icon:'package-check',stepIDs:['delivery'],stepTitles:['章节交付'],target:'deliveries',actionLabel:'查看章节交付'},
  ],
};
