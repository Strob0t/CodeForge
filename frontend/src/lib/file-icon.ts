import { getIconForFile, getIconForFolder, getIconForOpenFolder } from "vscode-icons-js";

const ICONS_BASE = "/icons/vscode/";

/** Returns the URL for a VS Code-style file/folder icon. */
export function fileIconUrl(name: string, isDir: boolean, isOpen = false): string {
  let svgName: string | undefined;
  if (isDir) {
    svgName = isOpen ? getIconForOpenFolder(name) : getIconForFolder(name);
  } else {
    svgName = getIconForFile(name);
  }
  return ICONS_BASE + (svgName ?? "default_file.svg");
}
