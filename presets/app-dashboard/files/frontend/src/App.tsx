import { AppShell } from "@hollis-labs/design-components"
import { NavRail, type NavRailItem, PageHeader, ThemeSwitcher } from "@hollis-labs/kit-dashboard"
import { Activity, LayoutDashboard } from "lucide-react"
import { useState } from "react"
import { DashboardPage } from "./pages/dashboard"

/**
 * App shell — the icon nav rail on the left, a pinned page header, and the
 * active page. Add pages by extending `nav` and the `route` switch below.
 */
export function App() {
  const [route, setRoute] = useState("dashboard")

  const nav: NavRailItem[] = [
    {
      key: "dashboard",
      label: "Dashboard",
      icon: <LayoutDashboard className="h-4 w-4" />,
      active: route === "dashboard",
      onSelect: () => setRoute("dashboard"),
    },
  ]

  return (
    <AppShell
      nav={
        <NavRail
          items={nav}
          logo={<Activity className="h-4 w-4" />}
          logoLabel="Dashboard"
          footerExtra={<ThemeSwitcher />}
        />
      }
      header={<PageHeader title="Dashboard" />}
    >
      <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-auto">
        <DashboardPage />
      </div>
    </AppShell>
  )
}
