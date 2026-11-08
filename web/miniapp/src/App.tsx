import { Spinner } from '@/ui/spinner'

export function App() {
  return (
    <main className="flex min-h-dvh flex-col items-center justify-center gap-3 p-4">
      <h1 className="text-lg font-semibold">Кабинет</h1>
      <Spinner />
    </main>
  )
}
