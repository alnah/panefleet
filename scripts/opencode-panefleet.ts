import { Plugin } from "@opencode-ai/plugin"

const defaultBridgePath = new URL("./opencode-event-bridge", import.meta.url).pathname
const bridgePath =
  process.env.PANEFLEET_OPENCODE_BRIDGE ||
  defaultBridgePath

export const PanefleetPlugin: Plugin = async () => {
  const pane = process.env.PANEFLEET_PANE || process.env.TMUX_PANE || ""

  return {
    event: async ({ event }) => {
      if (!pane) {
        return
      }

      const payload = JSON.stringify({ event })
      const proc = Bun.spawn([bridgePath, "--pane", pane], {
        stdin: "pipe",
        stdout: "ignore",
        stderr: "inherit",
      })

      if (proc.stdin) {
        await proc.stdin.write(payload)
        await proc.stdin.end()
      }
      await proc.exited
    },
  }
}
