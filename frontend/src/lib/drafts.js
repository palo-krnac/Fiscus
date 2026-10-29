// Koncept prijatej faktúry prečítaný z dokladu (AI) — prenesie sa zo zoznamu
// do editora a použije sa iba raz.
let pending = null

export function setPurchaseDraft(draft) {
  pending = draft
}

export function takePurchaseDraft() {
  const d = pending
  pending = null
  return d
}
