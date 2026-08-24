/**
 * @deprecated Use `../workbench/registry/registry` as the single workbench fact source.
 * This shim remains only for older imports while the Studio route migrates.
 */
import { getWorkbenchDefinition } from '../workbench/registry/registry';

export {
  getWorkbenchDefinition,
  type WorkbenchDefinition,
  type WorkbenchExperienceInput,
  type WorkbenchIconName,
  type WorkbenchNavigationItem,
  type WorkbenchPanelDefinition,
  type WorkbenchPanelTarget,
  type WorkbenchTone,
} from '../workbench/registry/registry';

export function resolveWorkbenchStepTitles(contentType:string,customTitles:string[]):string[] {
  const definition=getWorkbenchDefinition({content_type:contentType,step_titles:customTitles});
  const titles=customTitles.filter(title=>title.trim()).map(title=>title.trim());
  return titles.length>0?titles:definition.stepTitles;
}
