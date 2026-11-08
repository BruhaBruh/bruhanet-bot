import type { Me } from '../App'

// Заглушка: экран реализуется отдельной задачей плана.
export default function Friends({ me }: { me: Me }) {
  return <h2 className="title-3 p-4 font-semibold">{me.name || 'Друзья'}: Друзья</h2>
}
