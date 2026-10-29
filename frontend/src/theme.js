import { definePreset } from '@primeuix/themes'
import Aura from '@primeuix/themes/aura'

// Atramentová modrá — farba účtovných kníh. Kompaktné rozmery pre hustý ERP.
export const FiskusPreset = definePreset(Aura, {
  semantic: {
    primary: {
      50: '#eef3f8',
      100: '#d6e1ed',
      200: '#adc3db',
      300: '#7f9fc3',
      400: '#527ba8',
      500: '#2f5d8c',
      600: '#264d76',
      700: '#1f3a5c',
      800: '#192f4a',
      900: '#13233a',
      950: '#0c1726',
    },
    formField: {
      paddingX: '0.5rem',
      paddingY: '0.3125rem',
      borderRadius: '4px',
    },
    colorScheme: {
      light: {
        surface: {
          0: '#ffffff',
          50: '#f6f7f9',
          100: '#eef0f3',
          200: '#dce1e8',
          300: '#c5ccd6',
          400: '#98a2b1',
          500: '#6b7685',
          600: '#4e5866',
          700: '#38414d',
          800: '#262d36',
          900: '#1d2633',
          950: '#121820',
        },
      },
    },
  },
})

// Slovenská lokalizácia komponentov PrimeVue (kalendár, potvrdenia...)
export const skLocale = {
  accept: 'Áno',
  reject: 'Nie',
  cancel: 'Zrušiť',
  clear: 'Vymazať',
  apply: 'Použiť',
  today: 'Dnes',
  weekHeader: 'Týž',
  firstDayOfWeek: 1,
  dateFormat: 'd. m. yy',
  dayNames: ['nedeľa', 'pondelok', 'utorok', 'streda', 'štvrtok', 'piatok', 'sobota'],
  dayNamesShort: ['ne', 'po', 'ut', 'st', 'št', 'pi', 'so'],
  dayNamesMin: ['Ne', 'Po', 'Ut', 'St', 'Št', 'Pi', 'So'],
  monthNames: ['január', 'február', 'marec', 'apríl', 'máj', 'jún', 'júl', 'august', 'september', 'október', 'november', 'december'],
  monthNamesShort: ['jan', 'feb', 'mar', 'apr', 'máj', 'jún', 'júl', 'aug', 'sep', 'okt', 'nov', 'dec'],
  chooseYear: 'Vyberte rok',
  chooseMonth: 'Vyberte mesiac',
  chooseDate: 'Vyberte dátum',
  prevMonth: 'Predchádzajúci mesiac',
  nextMonth: 'Nasledujúci mesiac',
  emptyMessage: 'Žiadne výsledky',
  searchMessage: '{0} výsledkov',
  emptySearchMessage: 'Nič sa nenašlo',
  emptySelectionMessage: 'Nič nie je vybrané',
  emptyFilterMessage: 'Žiadne výsledky',
}
