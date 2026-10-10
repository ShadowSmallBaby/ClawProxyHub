import { api } from './client'
import type { Capabilities } from '@/features/policy'
export const capabilitiesApi = { get: () => api.get<Capabilities>('/admin/capabilities') }
