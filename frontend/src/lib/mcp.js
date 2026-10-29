// Návody na pripojenie AI asistenta k Fiškusu cez MCP.

/** Adresa MCP servera pre aktuálne otvorený Fiškus. */
export function mcpURL(origin = globalThis.location?.origin ?? 'http://127.0.0.1:8080') {
  return `${origin.replace(/\/$/, '')}/mcp`
}

/** Príkaz pre Claude Code (vzdialený HTTP server s hlavičkou). */
export function claudeCodeCommand(url, token) {
  return `claude mcp add --transport http fiskus ${url} \\\n  --header "Authorization: Bearer ${token}"`
}

/** Konfigurácia pre aplikácie, ktoré spúšťajú MCP servery cez stdio (napr. Claude Desktop). */
export function stdioConfig(url, token) {
  return JSON.stringify(
    {
      mcpServers: {
        fiskus: { command: 'fiskus-mcp', args: ['-url', url], env: { FISKUS_API_KEY: token } },
      },
    },
    null,
    2,
  )
}
