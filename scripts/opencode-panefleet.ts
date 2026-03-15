import { Plugin } from "@opencode-ai/plugin"

const bridgePath = process.env.PANEFLEET_OPENCODE_BRIDGE || "/opt/homebrew/Cellar/panefleet/0.3.3/libexec/scripts/opencode-event-bridge"

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
