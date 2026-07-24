import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from 'react'

export interface Toast {
  id: number
  message: string
  kind: 'success' | 'error'
}

interface ToastContextValue {
  toasts: Toast[]
  showToast: (message: string, kind?: Toast['kind']) => void
  dismissToast: (id: number) => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

const TOAST_LIFETIME_MS = 4000

// A global toast is the fix for "did my StatusNotification/Start/etc. even
// go out?" — every action panel used to only clear its own busy state on
// success with no visible confirmation, so the only way to check was
// scrolling down to the event log. A toast fires exactly when the action
// completes, wherever the operator's eyes currently are.
export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
  const nextID = useRef(0)

  const dismissToast = useCallback((id: number) => {
    setToasts((current) => current.filter((toast) => toast.id !== id))
  }, [])

  const showToast = useCallback((message: string, kind: Toast['kind'] = 'success') => {
    const id = nextID.current++
    setToasts((current) => [...current, { id, message, kind }])
    setTimeout(() => dismissToast(id), TOAST_LIFETIME_MS)
  }, [dismissToast])

  return (
    <ToastContext.Provider value={{ toasts, showToast, dismissToast }}>
      {children}
    </ToastContext.Provider>
  )
}

export function useToast() {
  const context = useContext(ToastContext)
  if (!context) throw new Error('useToast must be used within ToastProvider')
  return context
}
