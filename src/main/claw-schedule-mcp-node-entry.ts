import { runClawScheduleMcpServerFromArgv } from './claw-schedule-mcp-server'
import { publicConsoleError } from './logger'

void runClawScheduleMcpServerFromArgv(process.argv)
  .then((handled) => {
    if (handled) return
    publicConsoleError('claw-schedule-mcp', 'Missing required server launch flag.')
    process.exit(1)
  })
  .catch((error) => {
    publicConsoleError('claw-schedule-mcp', 'Server failed.', error)
    process.exit(1)
  })
