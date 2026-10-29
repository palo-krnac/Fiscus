import { describe, expect, it } from 'vitest'
import { claudeCodeCommand, connectorReady, isOAuthGrant, mcpURL, stdioConfig } from './mcp.js'

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

describe('konektor pre Claude na webe', () => {
  it.each([
    ['https://fiskus.kancelaria.sk/mcp', true],
    ['http://fiskus.kancelaria.sk/mcp', false],
    ['https://localhost:8080/mcp', false],
    ['https://192.168.1.10/mcp', false],
    ['https://172.20.0.5/mcp', false],
    ['nie-je-adresa', false],
  ])('%s', (url, ok) => expect(connectorReady(url)).toBe(ok))
  it('povolenie OAuth', () => {
    expect(isOAuthGrant({ prefix: 'OAuth' })).toBe(true)
    expect(isOAuthGrant({ prefix: 'fk_ab12cd' })).toBe(false)
  })
})
