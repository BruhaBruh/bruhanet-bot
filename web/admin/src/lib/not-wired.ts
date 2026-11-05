import { toast } from '@/ui/toast'

export const NOT_WIRED = 'Пока не подключено'

/** Действие из макета без бэкенда. Поля, меняющие то, что уйдёт, — не сюда: они disabled с подсказкой NOT_WIRED. */
export const notWired = () => toast(NOT_WIRED)
