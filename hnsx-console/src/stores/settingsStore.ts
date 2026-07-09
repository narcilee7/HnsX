import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface SettingsState {
  sidebarCollapsed: boolean
  toggleSidebar: () => void
  theme: 'light' | 'dark' | 'system'
  setTheme: (theme: 'light' | 'dark' | 'system') => void
  /** Grafana base URL override — 优先级高于环境变量 */
  grafanaUrlOverride: string | null
  setGrafanaUrlOverride: (url: string | null) => void
}

export const useSettingsStore = create<SettingsState>()(
  persist(
    (set) => ({
      sidebarCollapsed: false,
      toggleSidebar: () => set((state) => ({ sidebarCollapsed: !state.sidebarCollapsed })),
      theme: 'system',
      setTheme: (theme) => set({ theme }),
      grafanaUrlOverride: null,
      setGrafanaUrlOverride: (url) => set({ grafanaUrlOverride: url }),
    }),
    {
      name: 'hnsx-settings',
    },
  ),
)