/* @refresh reload */
import { render } from "solid-js/web";
import { Route, Router } from "@solidjs/router";
import App from "./App.tsx";
import DashboardPage from "./features/dashboard/DashboardPage.tsx";
import "./index.css";

const root = document.getElementById("root");

if (!root) {
  throw new Error("Root element not found");
}

render(
  () => (
    <Router root={App}>
      <Route path="/" component={DashboardPage} />
      <Route path="/projects" component={DashboardPage} />
    </Router>
  ),
  root,
);
