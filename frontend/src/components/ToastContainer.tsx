import { useToast } from '../state/ToastContext'

export function ToastContainer() {
  const { toasts, dismissToast } = useToast()
  if (toasts.length === 0) return null

  return (
    <div className="toast-stack">
      {toasts.map((toast) => (
        <div key={toast.id} className={`toast toast-${toast.kind}`} onClick={() => dismissToast(toast.id)}>
          {toast.message}
        </div>
      ))}
    </div>
  )
}
