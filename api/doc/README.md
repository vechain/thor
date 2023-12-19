## Stoplight
Spotlight UI from https://github.com/stoplightio/elements
 - Modified [web-components.min.js](api-docs/web-components.min.js) so that websocket endpoints do not show examples.

```diff
- t && r.createElement(li,{"data-testid":"two-column-right"....})
+ t && !window.location.href.includes("paths/subscriptions") && r.createElement(li,{"data-testid":"two-column-right"....})
```
