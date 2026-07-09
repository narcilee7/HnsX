import { create } from 'zustand'
import { persist } from 'zustand/middleware'

interface SettingsState {
  sidebarCollapsed: boolean
  toggleSidebar: () => void
  theme: 'light' | 'dark' | 'system'
  setTheme: (theme: 'light' | 'dark' | 'system') => void
}

export const useSettingsStore = create<SettingsState>()(
  persist(
    (set) => ({
      sidebarCollapsed: false,
      toggleSidebar: () => set((state) => ({ sidebarCollapsed: !state.sidebarCollapsed })),
      theme: 'system',
      setTheme: (theme) => set({ theme }),
    }),
    {
      name: 'hnsx-settings',
    },
  ),
)
