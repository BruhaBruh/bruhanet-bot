import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// без globals: true RTL сам не чистит DOM между тестами
afterEach(cleanup)
