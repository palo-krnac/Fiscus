// Dvojfázové overenie: pomocné funkcie pre prihlásenie a nastavenie.
import QRCode from 'qrcode'

/** Kľúč po štvoriciach, aby sa dal opísať do aplikácie ručne. */
export function formatSecret(secret) {
  return (secret || '').replace(/\s+/g, '').match(/.{1,4}/g)?.join(' ') ?? ''
}

/** Kód z aplikácie má 6 číslic (medzery sa ignorujú). */
export function isAppCode(code) {
  return /^\d{6}$/.test((code || '').replace(/\s+/g, ''))
}

/** Text súboru so záložnými kódmi. */
export function recoveryFile(codes, email, date = new Date()) {
  return [
    'Fiškus – záložné kódy pre dvojfázové overenie',
    `Účet: ${email}`,
    `Vytvorené: ${date.toLocaleDateString('sk-SK')}`,
    '',
    'Každý kód platí iba raz. Použite ho, keď nemáte pri sebe telefón.',
    'Uschovajte ich mimo počítača, napríklad vytlačené.',
    '',
    ...codes,
    '',
  ].join('\n')
}

/** QR kód adresy otpauth:// ako SVG. */
export function mfaQR(uri) {
  return QRCode.toString(uri, { type: 'svg', errorCorrectionLevel: 'M', margin: 0 })
}
