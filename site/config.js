// Nastavenie webu fiskus.sk. Predaj je vypnutý, kým nie je vyplnené paddle.token
// a paddle.priceId (Paddle → Developer tools → Authentication / Catalog).
window.FISKUS = {
  version: '0.1.0',
  // kde ležia súbory na stiahnutie (napr. https://fiskus.sk/stiahnut/0.1.0/)
  downloads: 'stiahnut/0.1.0/',
  paddle: {
    environment: 'sandbox', // 'sandbox' na skúšku, 'production' naostro
    token: '', // client-side token (test_… / live_…)
    priceId: '', // cena „Server – viac firiem“, 99 € ročne (pri_…)
  },
  contact: 'info@fiskus.sk',
}
