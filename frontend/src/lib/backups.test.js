import { describe, expect, it } from 'vitest'
import { changeNotes, cloudLabel, confirmError } from './backups.js'

describe('confirmError', () => {
  it.each([
    ['', '', ''],
    ['heslo123', 'heslo123', ''],
    ['heslo123', 'heslo12', 'Heslá sa nezhodujú'],
  ])('%s / %s', (p, c, want) => expect(confirmError(p, c)).toBe(want))
})

describe('changeNotes', () => {
  it('upozorní iba pri existujúcich zálohách', () => {
    expect(changeNotes({ count: 0, passwordSet: true, dirChanged: true, passwordChanged: true })).toEqual([])
    expect(changeNotes({ count: 3, passwordSet: false, dirChanged: false, passwordChanged: true })).toEqual([])
    expect(changeNotes({ count: 3, passwordSet: true, dirChanged: true, passwordChanged: true })).toHaveLength(2)
  })
})

describe('cloudLabel', () => {
  it.each([
    ['C:\\Users\\palo\\OneDrive - Firma\\Fiškus zálohy', 'OneDrive - Firma'],
    ['/home/palo/Dropbox/Fiškus zálohy', 'Dropbox'],
    ['/Users/p/Google Drive/My Drive/Fiškus zálohy', 'Google Disk'],
    ['/Users/p/Library/Mobile Documents/com~apple~CloudDocs/Fiškus zálohy', 'iCloud'],
  ])('%s', (p, want) => expect(cloudLabel(p)).toBe(want))
})
