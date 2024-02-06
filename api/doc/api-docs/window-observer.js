/**
 * This script is used to remove the "Send API request" button and the "Response" sections from WebSocket operations.
 * @type {MutationObserver}
 */
const mutationObserver = new MutationObserver(() => {
    if (window.location.hash?.includes("#/paths/subscriptions")) {
        const element = document.querySelector('[data-testid="two-column-right"]');
        if (element) {
            element.remove();
        }
    }
})

mutationObserver.observe(document, {attributes: false, childList: true, characterData: false, subtree:true});
