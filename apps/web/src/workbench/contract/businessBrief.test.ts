import { describe, expect, it } from 'vitest';
import { buildBusinessBrief, businessBriefReady, emptyBusinessBriefForm } from './businessBrief';

describe('business brief contract',()=>{
  it('builds a normalized video or article brief without owning a workflow state',()=>{
    const value={...emptyBusinessBriefForm(),audience:' 新用户 ',channel:'公众号',tone:'可信',keywords:'成分, 成分\n注意事项'};
    expect(buildBusinessBrief('wechat-article',value)).toEqual({schema_version:'contentcloud.business-brief/1.0',audience:'新用户',channel:'公众号',tone:'可信',keywords:['成分','成分','注意事项']});
    expect(businessBriefReady('article',value)).toBe(true);
  });

  it('parses commerce facts and keeps unknown business types extensible',()=>{
    const value={...emptyBusinessBriefForm(),channel:'短视频',targetAudience:'上班族',productFacts:'净含量: 500g\n: 忽略\n产地: 中国',offerPoints:'低糖\n独立包装'};
    expect(buildBusinessBrief('commerce',value)).toEqual({schema_version:'contentcloud.business-brief/1.0',channel:'短视频',target_audience:'上班族',product_facts:{'净含量':'500g','产地':'中国'},offer_points:['低糖','独立包装']});
    expect(businessBriefReady('future_business',emptyBusinessBriefForm())).toBe(true);
    expect(buildBusinessBrief('future_business',emptyBusinessBriefForm())).toBeUndefined();
  });
});
