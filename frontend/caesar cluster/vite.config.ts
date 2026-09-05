import { defineConfig } from 'vite'
import react, { reactCompilerPreset } from '@vitejs/plugin-react'
import babel from '@rolldown/plugin-babel'
import tailwindcss from '@tailwindcss/vite'
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  // .env มีไฟล์เดียวอยู่ที่ root ของ repo ไม่ใช่ในโฟลเดอร์ frontend นี้
  // (ตอนรันใน docker ค่าถูกส่งมาทาง env ของ container อยู่แล้ว ไม่ได้พึ่ง path นี้)
  envDir: path.resolve(__dirname, '../..'),
  plugins: [
    react(),
    babel({ presets: [reactCompilerPreset()] }),
    tailwindcss(), 
  ],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      'react': path.resolve(__dirname, 'node_modules/react'),
      'react-dom': path.resolve(__dirname, 'node_modules/react-dom'),
    },
  },
})