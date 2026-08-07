import { describe, expect, it } from 'vitest';
import { parseCSV } from './StudioAssetDetailPage';

describe('asset table renderer',()=>{
  it('parses quoted commas, escaped quotes, and line breaks',()=>{
    expect(parseCSV('镜头,旁白\r\n01,"城市,醒来"\r\n02,"他说""你好"""')).toEqual([
      ['镜头','旁白'],
      ['01','城市,醒来'],
      ['02','他说"你好"'],
    ]);
  });
});
