import type { Me } from '../App'

// Заглушка: экран реализуется отдельной задачей плана.
export default function Promos({ me }: { me: Me }) {
  return <h2 className="title-3 p-4 font-semibold">{me.name || 'Промокоды'}: Промокоды</h2>
}
