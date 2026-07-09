import { create } from 'zustand'
import type { ReactNode } from 'react'

export interface Notification {
  id: string
  title: string
  description?: ReactNode
  type?: 'info' | 'success' | 'warning' | 'error'
}

interface NotificationState {
  notifications: Notification[]
  addNotification: (notification: Omit<Notification, 'id'>) => void
  removeNotification: (id: string) => void
}

export const useNotificationStore = create<NotificationState>((set) => ({
  notifications: [],
  addNotification: (notification) => {
    const id = Math.random().toString(36).slice(2)
    set((state) => ({
      notifications: [...state.notifications, { ...notification, id }],
    }))
    return id
  },
  removeNotification: (id) =>
    set((state) => ({
      notifications: state.notifications.filter((n) => n.id !== id),
    })),
}))
