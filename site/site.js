// fiskus.sk: odkazy na stiahnutie podľa systému a nákup licencie cez Paddle.
;(function () {
  const cfg = window.FISKUS || {}
  const v = cfg.version || ''

  // ---- stiahnutie ----
  const files = {
    windows: `fiskus-${v}-windows-setup.exe`,
    macos: `fiskus-${v}-macos.dmg`,
    deb: `fiskus_${v}_amd64.deb`,
    rpm: `fiskus-${v}-1.x86_64.rpm`,
  }
  document.querySelectorAll('[data-file]').forEach((a) => {
    const name = files[a.dataset.file]
    a.href = (cfg.downloads || '') + name
    a.setAttribute('download', '')
    const label = document.querySelector(`[data-name="${a.dataset.file}"]`)
    if (label) label.textContent = name
  })
  document.querySelectorAll('[data-version]').forEach((el) => (el.textContent = v))

  const ua = navigator.userAgent
  const os = /Windows/.test(ua) ? 'windows' : /Mac OS X/.test(ua) && !/iPhone|iPad/.test(ua) ? 'macos' : /Linux/.test(ua) && !/Android/.test(ua) ? 'linux' : ''
  if (os) {
    document.querySelector(`tr[data-os="${os}"]`)?.classList.add('mine')
    const hero = document.querySelector('[data-hero-download]')
    if (hero) {
      const target = os === 'linux' ? 'deb' : os
      hero.href = (cfg.downloads || '') + files[target]
      hero.setAttribute('download', '')
      hero.textContent = { windows: 'Stiahnuť pre Windows', macos: 'Stiahnuť pre macOS', linux: 'Stiahnuť pre Linux' }[os]
    }
  }

  // ---- nákup licencie ----
  const form = document.querySelector('#kupit-form')
  if (!form) return
  const msg = form.querySelector('.msg')
  const p = cfg.paddle || {}
  const ready = !!(p.token && p.priceId)
  const button = form.querySelector('button')
  if (!ready) {
    button.disabled = true
    msg.textContent = `Online predaj spúšťame. Licenciu zatiaľ objednáte e-mailom na ${cfg.contact}.`
    return
  }

  let loaded = null
  function loadPaddle() {
    loaded ??= new Promise((resolve, reject) => {
      const s = document.createElement('script')
      s.src = 'https://cdn.paddle.com/paddle/v2/paddle.js'
      s.onload = () => {
        if (p.environment === 'sandbox') window.Paddle.Environment.set('sandbox')
        window.Paddle.Initialize({
          token: p.token,
          eventCallback: (e) => {
            if (e.name === 'checkout.completed') {
              msg.classList.remove('err')
              msg.textContent = 'Ďakujeme. Licenčný kľúč príde do niekoľkých minút na zadaný e-mail.'
            }
          },
        })
        resolve(window.Paddle)
      }
      s.onerror = () => reject(new Error('Platobnú bránu sa nepodarilo načítať. Skúste to znova.'))
      document.head.appendChild(s)
    })
    return loaded
  }

  form.addEventListener('submit', async (e) => {
    e.preventDefault()
    const company = form.company.value.trim()
    const email = form.email.value.trim()
    if (!company || !form.email.checkValidity()) {
      msg.classList.add('err')
      msg.textContent = 'Zadajte obchodné meno a platný e-mail — pošleme naň licenčný kľúč.'
      return
    }
    msg.classList.remove('err')
    msg.textContent = ''
    button.disabled = true
    try {
      const paddle = await loadPaddle()
      // obchodné meno a e-mail dostane licenčný server v custom_data
      paddle.Checkout.open({
        items: [{ priceId: p.priceId, quantity: 1 }],
        customer: { email },
        customData: { company, email },
        settings: { displayMode: 'overlay' },
      })
    } catch (err) {
      msg.classList.add('err')
      msg.textContent = err.message
    } finally {
      button.disabled = false
    }
  })
})()
