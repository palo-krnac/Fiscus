import { describe, expect, it } from 'vitest'
import { blankTrip, filterFuel, filterTrips, fuelUnit, logbookPosting, methodHint, methodOptions, purposeSuggestions, returnTrip, tripRoute, tripTime, vehicleLabel } from './logbook.js'

const trips = [
  { id: 1, date: '2026-10-01', from: 'Banská Bystrica', to: 'Zvolen', purpose: 'Montáž rozvádzača', kind: 'business', km: 25, driver: 'Pavol' },
  { id: 2, date: '2026-10-02', from: 'Banská Bystrica', to: 'Žilina', purpose: 'Obhliadka', kind: 'business', km: 110 },
  { id: 3, date: '2026-10-03', from: 'Zvolen', to: 'Tesco', purpose: '', kind: 'private', km: 8 },
  { id: 4, date: '2026-10-04', from: 'BB', to: 'ZV', purpose: 'Montáž rozvádzača', kind: 'business', km: 25 },
]

describe('kniha jázd', () => {
  it.each([
    ['', 'all', [1, 2, 3, 4]],
    ['zilina', 'all', [2]],
    ['montaz', 'business', [1, 4]],
    ['', 'private', [3]],
    ['110', 'all', [2]],
    ['1. 10. 2026', 'all', [1]],
  ])('hľadanie „%s“ (%s)', (q, kind, want) => expect(filterTrips(trips, q, kind).map((t) => t.id)).toEqual(want))

  it('tankovanie podľa stanice a dokladu', () => {
    const fuel = [{ id: 1, date: '2026-10-01', station: 'Slovnaft Zvolen', docNumber: 'VPD20260007' }, { id: 2, date: '2026-10-05', station: 'OMV' }]
    expect(filterFuel(fuel, 'slovnaft').map((f) => f.id)).toEqual([1])
    expect(filterFuel(fuel, 'vpd2026').map((f) => f.id)).toEqual([1])
  })

  it('popisy', () => {
    expect(tripRoute(trips[0])).toBe('Banská Bystrica – Zvolen')
    expect(tripTime({ depart: '07:30', arrive: '08:10' })).toBe('07:30–08:10')
    expect(tripTime({})).toBe('')
    expect(vehicleLabel({ plate: 'BB123AB', name: 'Octavia', archived: true })).toBe('BB123AB Octavia – archivované')
    expect(fuelUnit('electric')).toBe('kWh')
    expect(methodOptions('private')[1].label).toBe('Paušál 50 % PHM')
    expect(methodOptions('business')[1].label).toBe('Paušál 80 % PHM')
    expect(methodHint({ ownership: 'private', method: 'log' })).toContain('základná náhrada')
  })

  it('účely podľa častosti', () => {
    expect(purposeSuggestions(trips)).toEqual(['Montáž rozvádzača', 'Obhliadka'])
  })

  it('nová a spiatočná jazda', () => {
    const t = blankTrip({ id: 5, driver: 'Pavol', lastOdometer: 12040 }, '2026-10-30')
    expect([t.vehicleId, t.odoStart, t.driver, t.kind]).toEqual([5, 12040, 'Pavol', 'business'])
    const back = returnTrip({ id: 9, from: 'BB', to: 'ZV', odoStart: 100, odoEnd: 125, km: 25, purpose: 'Montáž' })
    expect([back.id, back.from, back.to, back.odoStart, back.odoEnd, back.purpose]).toEqual([0, 'ZV', 'BB', 125, 150, 'Montáž'])
  })

  it.each([
    [{ posting: '0', posted: '0' }, 'draft', ''],
    [{ posting: '-30.00', posted: '0' }, 'overdue', 'Zaúčtovať do uzávierky'],
    [{ posting: '-30.00', posted: '-25.00' }, 'overdue', 'Zaúčtovať znova'],
    [{ posting: '102.10', posted: '102.10' }, 'paid', 'Zaúčtovať znova'],
  ])('zaúčtovanie %#', (r, tone, action) => {
    const s = logbookPosting(r)
    expect([s.tone, s.action]).toEqual([tone, action])
  })
})
