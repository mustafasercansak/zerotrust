import { trTR, enUS } from "@mui/x-data-grid/locales";

/**
 * Returns the MUI DataGrid locale bundle based on the active i18next language identifier.
 * Easily add future languages (e.g. frFR, deDE) by importing them and adding another branch.
 */
export function getMuiDataGridLocale(language?: string) {
  if (!language) return enUS;
  if (language.startsWith("tr")) return trTR;
  // Future languages:
  // if (language.startsWith("fr")) return frFR;
  // if (language.startsWith("de")) return deDE;
  return enUS;
}
