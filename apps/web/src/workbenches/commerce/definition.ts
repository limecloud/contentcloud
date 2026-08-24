import type { WorkbenchDefinition } from '../../workbench/registry/registry';
import { navigation } from '../../workbench/contract/navigation';

export const commerceWorkbench:WorkbenchDefinition={
  key:'commerce',contentTypes:['commerce','ecommerce','douyin_commerce','product_content'],eyebrow:'电商内容工作台',sourceTitle:'选择商品与素材',sourceDetail:'从商品信息、卖点资料和已确认内容开始制作渠道版本',
  navigation:[navigation('overview','商品首页','shopping-bag'),navigation('tasks','商品任务','archive'),navigation('catalog','商品资料','tags'),navigation('deliveries','渠道交付','package-check')],
  stepTitles:['商品信息','卖点策略','内容变体','渠道预览','交付'],
  panels:[
    {id:'product',title:'商品与卖点',detail:'整理商品事实、目标人群和可验证的购买理由。',tone:'source',icon:'shopping-bag',stepIDs:['product','brief','offer'],stepTitles:['商品信息','卖点策略'],target:'start',actionLabel:'开始商品策划'},
    {id:'variants',title:'内容变体',detail:'围绕同一商品生成不同渠道和场景需要的内容版本。',tone:'strategy',icon:'tags',stepIDs:['variants','script','copy'],stepTitles:['内容变体'],target:'tasks',actionLabel:'查看内容变体'},
    {id:'preview',title:'渠道预览',detail:'对照渠道规格检查标题、卖点、图片和行动引导。',tone:'production',icon:'image',stepIDs:['preview','channel'],stepTitles:['渠道预览'],target:'tasks',actionLabel:'查看渠道预览'},
    {id:'delivery',title:'交付包',detail:'确认版本和文件清单，按渠道导出可交接内容。',tone:'review',icon:'package-check',stepIDs:['delivery','publish'],stepTitles:['交付'],target:'deliveries',actionLabel:'查看渠道交付'},
  ],
};
