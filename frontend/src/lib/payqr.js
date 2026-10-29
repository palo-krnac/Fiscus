// QR platba PAY by square (štandard Slovenskej bankovej asociácie).
// Obsah QR kódu sa skladá z hotových súm z API — nič sa tu neprepočítava.
import { CurrencyCode, PaymentOptions, encode } from 'bysquare/pay'
import QRCode from 'qrcode'

/**
 * Vráti dátový model platby pre vystavenú faktúru, alebo null, ak QR nedáva zmysel
 * (koncept, storno, uhradená, nulová/záporná suma, chýba IBAN).
 */
export function payModel(invoice, company) {
  if (!invoice || !company?.iban) return null
  if (invoice.status !== 'issued') return null
  if (invoice.paymentMethod && invoice.paymentMethod !== 'transfer') return null
  const total = invoice.summary?.total
  if (!total || !(Number(total) > 0)) return null

  const iban = company.iban.replace(/\s/g, '').toUpperCase()
  return {
    invoiceId: (invoice.number || '').slice(0, 10),
    payments: [
      {
        type: PaymentOptions.PaymentOrder,
        // bysquare chce číslo; suma má najviac 2 desatinné miesta, takže je presná
        amount: Number(total),
        currencyCode: CurrencyCode[invoice.currency] ?? CurrencyCode.EUR,
        paymentDueDate: invoice.dueDate ? invoice.dueDate.replaceAll('-', '') : undefined,
        variableSymbol: invoice.variableSymbol || undefined,
        paymentNote: `Faktura ${invoice.number}`.slice(0, 140),
        bankAccounts: [{ iban }],
        beneficiary: {
          name: company.name.slice(0, 70),
          street: company.street ? company.street.slice(0, 70) : undefined,
          city: [company.zip, company.city].filter(Boolean).join(' ').slice(0, 70) || undefined,
        },
      },
    ],
  }
}

/** Reťazec PAY by square (bez diakritiky — niektoré banky ju nezvládajú). */
export function payBySquare(invoice, company) {
  const model = payModel(invoice, company)
  return model ? encode(model, { deburr: true, validate: true }) : null
}

/** SVG QR kódu (bez okrajov, úroveň opravy chýb M). */
export function qrSvg(text) {
  return QRCode.toString(text, { type: 'svg', errorCorrectionLevel: 'M', margin: 0 })
}
