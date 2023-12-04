import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { library } from '@fortawesome/fontawesome-svg-core'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import { faAngleDown, faAngleUp, faAnglesLeft, faAnglesRight, faDiagramProject, faFile, faFileCircleXmark, faHouse, faTable } from '@fortawesome/free-solid-svg-icons'

// CSS
import 'vue-multiselect/dist/vue-multiselect.css'
import './assets/main.css'
import './assets/multiselect.css'

// Fontawesome
library.add(faAngleDown, faAngleUp, faAnglesLeft, faAnglesRight, faDiagramProject, faFile, faFileCircleXmark, faHouse, faTable)

// Create app
createApp(App)
    .use(router)
    .component('font-awesome-icon', FontAwesomeIcon)
    .mount('#app')
