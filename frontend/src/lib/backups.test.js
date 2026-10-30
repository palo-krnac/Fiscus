import { describe, expect, it } from 'vitest'
import { changeNotes, cloudLabel, confirmError, remoteStateText, remoteTarget, s3Endpoint, s3ProviderOf } from './backups.js'

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

describe('s3Endpoint', () => {
  it.each([
    ['aws', 'eu-central-1', 'https://s3.eu-central-1.amazonaws.com'],
    ['wasabi', 'eu-central-2', 'https://s3.eu-central-2.wasabisys.com'],
    ['b2', 'eu-central-003', 'https://s3.eu-central-003.backblazeb2.com'],
    ['hetzner', 'fsn1', 'https://fsn1.your-objectstorage.com'],
    ['aws', ' ', ''],
    ['other', 'eu-central-1', ''],
  ])('%s %s', (p, r, want) => expect(s3Endpoint(p, r)).toBe(want))

  it('rozpozná poskytovateľa z uloženej adresy', () => {
    expect(s3ProviderOf('https://fsn1.your-objectstorage.com', 'fsn1')).toBe('hetzner')
    expect(s3ProviderOf('http://127.0.0.1:9000', 'us-east-1')).toBe('other')
    expect(s3ProviderOf('', '')).toBe('aws')
  })
})

describe('remoteTarget', () => {
  it.each([
    [{ kind: '' }, ''],
    [{ kind: 'webdav', url: 'https://cloud.firma.sk/remote.php/dav/files/jana/Fi%C5%A1kus/' }, 'WebDAV cloud.firma.sk/remote.php/dav/files/jana/Fiškus'],
    [{ kind: 's3', url: 'https://s3.eu-central-1.amazonaws.com', bucket: 'firma', prefix: 'fiskus' }, 'S3 s3.eu-central-1.amazonaws.com · firma/fiskus'],
    [{ kind: 's3', url: 'https://s3.eu-central-1.amazonaws.com', bucket: 'firma', prefix: '' }, 'S3 s3.eu-central-1.amazonaws.com · firma'],
  ])('%j', (r, want) => expect(remoteTarget(r)).toBe(want))
})

describe('remoteStateText', () => {
  it('rozlíši úspech, chybu a čakanie', () => {
    expect(remoteStateText({ count: 0 }).tone).toBe('wait')
    const ok = remoteStateText({ lastOk: '2026-10-29T17:00:00Z', count: 3 })
    expect(ok.tone).toBe('ok')
    expect(ok.text).toContain('3 zálohy')
    expect(remoteStateText({ lastOk: '2026-10-29T17:00:00Z', count: 5 }).text).toContain('5 záloh')
    const bad = remoteStateText({ lastOk: '2026-10-28T17:00:00Z', count: 1, lastError: 'WebDAV nahratie: prístup zamietnutý (401)', errorAt: '2026-10-29T17:00:00Z' })
    expect(bad.tone).toBe('error')
    expect(bad.text).toContain('prístup zamietnutý')
    // staršia chyba po novšom úspechu sa neukazuje
    expect(remoteStateText({ lastOk: '2026-10-30T17:00:00Z', count: 1, lastError: 'x', errorAt: '2026-10-29T17:00:00Z' }).tone).toBe('ok')
  })
})
