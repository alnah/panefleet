import { Plugin } from "@opencode-ai/plugin"

const bridgePath =
  process.env.PANEFLEET_OPENCODE_BRIDGE ||
  `${process.env.HOME}/.tmux/plugins/panefleet/scripts/opencode-event-bridge`

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
        stderr: "ignore",
      })

      const writer = proc.stdin.getWriter()
      await writer.write(new TextEncoder().encode(payload))
      await writer.close()
      await proc.exited
    },
  }
}
