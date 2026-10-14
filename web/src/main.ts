import { mount } from 'svelte'
import '@fontsource/jetbrains-mono/600.css'
import '@fontsource/jetbrains-mono/700.css'
import './app.css'
import App from './App.svelte'

mount(App, { target: document.getElementById('app')! })
