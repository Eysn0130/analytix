import { z } from 'zod'

/** Local UI diagnostics only; never an authority or model-routing input. */
export const desktopEnvironmentSchema = z.discriminatedUnion('mode', [
  z.object({ mode: z.literal('packaged') }).strict(),
  z.object({ mode: z.literal('browser') }).strict(),
  z.object({
    mode: z.literal('development'),
    profileId: z.string().regex(/^[a-f0-9]{12}$/),
    isolated: z.boolean(),
    mock: z.boolean(),
    version: z.string().max(80),
    mainBuildId: z.string().regex(/^[a-f0-9]{12}$/).optional()
  }).strict()
])

export type DesktopEnvironment = z.infer<typeof desktopEnvironmentSchema>
