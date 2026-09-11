import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import enCommon from './locales/en/common.json'
import zhCommon from './locales/zh/common.json'
import enSettings from './locales/en/settings.json'
import zhSettings from './locales/zh/settings.json'

const initialLanguage = import.meta.env.MODE === 'test' ? 'en' : 'zh'

void i18n.use(initReactI18next).init({
  resources: {
    en: { common: enCommon, settings: enSettings },
    zh: { common: zhCommon, settings: zhSettings }
  },
  lng: initialLanguage,
  fallbackLng: initialLanguage,
  initImmediate: false,
  interpolation: { escapeValue: false },
  defaultNS: 'common',
  ns: ['common', 'settings']
})

export default i18n
