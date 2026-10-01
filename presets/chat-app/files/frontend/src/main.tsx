import React from "react"
import ReactDOM from "react-dom/client"
import { App } from "./App"
import { ApiProvider } from "./api/context"
import "./index.css"

const root = document.getElementById("root")
if (!root) {
  throw new Error("root element #root not found")
}

ReactDOM.createRoot(root).render(
  <React.StrictMode>
    <ApiProvider>
      <App />
    </ApiProvider>
  </React.StrictMode>,
)
