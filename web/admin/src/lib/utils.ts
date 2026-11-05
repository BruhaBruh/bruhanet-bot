import { type ClassValue, clsx } from 'clsx'
import { extendTailwindMerge } from 'tailwind-merge'

// Токены DS, о которых tailwind-merge не знает. Типографика — своя группа: font-* и text-* её не отменяют.
const twMerge = extendTailwindMerge<'typography'>({
  extend: {
    theme: { radius: ['sheet', 'bubble'], shadow: ['card', 'popover', 'thumb'] },
    classGroups: {
      typography: [
        'large-title',
        'title-1',
        'title-2',
        'title-3',
        'headline',
        'body',
        'callout',
        'subheadline',
        'footnote',
        'caption-1',
        'caption-2',
        'admin-page-title',
        'admin-body',
        'admin-table',
        'admin-label',
        'admin-stat',
        'mono',
      ],
    },
  },
})

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
