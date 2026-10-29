import { describe, expect, it } from 'vitest'
import { claudeCodeCommand, mcpURL, stdioConfig } from './mcp.js'

describe('pripojenie AI', () => {
  it('adresa a príkazy', () => {
    const url = mcpURL('http://127.0.0.1:8080/')
    expect(url).toBe('http://127.0.0.1:8080/mcp')
    expect(claudeCodeCommand(url, 'fk_x')).toContain('claude mcp add --transport http fiskus http://127.0.0.1:8080/mcp')
    expect(claudeCodeCommand(url, 'fk_x')).toContain('Authorization: Bearer fk_x')
    const cfg = JSON.parse(stdioConfig(url, 'fk_x'))
    expect(cfg.mcpServers.fiskus.env.FISKUS_API_KEY).toBe('fk_x')
    expect(cfg.mcpServers.fiskus.args).toEqual(['-url', url])
  })
})
