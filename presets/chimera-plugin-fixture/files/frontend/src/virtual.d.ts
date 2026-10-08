declare module "virtual:plugin-host-ui/stylesheets" {
  export function createStylesheetLeases(
    document: Document,
  ): import("@hollis-labs/plugin-host-ui/vite").StylesheetLeases
}
