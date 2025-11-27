/**
 * This script is used to:
 * 1. Remove the "Send API request" button and the "Response" sections from WebSocket operations.
 * 2. Format API responses to be more human-readable (hex to decimal, timestamps, etc.)
 */

const vetKeys = ['balance', 'totalSupply', 'amount', 'value'];
const decimalKeys = ['maxFeePerGas', 'paid', 'reward', 'gasPrice', 'baseFee', 'baseFeePerGas', 'maxPriorityFeePerGas'];
const timestampKeys = ['timestamp', 'time', 'blockTimestamp'];


// Utility functions for formatting
const formatters = {
    /**
     * Convert hex string to decimal number
     */
    hexToDecimal: (hex) => {
        if (!hex || typeof hex !== 'string' || !hex.startsWith('0x')) {
            return hex;
        }
        try {
            return BigInt(hex).toString();
        } catch (e) {
            return hex;
        }
    },

    /**
     * Convert wei to VET (1 VET = 10^18 wei)
     */
    weiToVET: (weiHex) => {
        if (!weiHex || typeof weiHex !== 'string' || !weiHex.startsWith('0x')) {
            return weiHex;
        }
        try {
            const wei = BigInt(weiHex);
            const vet = Number(wei) / 1e18;
            return vet.toFixed(6) + ' VET';
        } catch (e) {
            return weiHex;
        }
    },

    /**
     * Convert wei to VTHO (1 VTHO = 10^18 wei)
     */
    weiToVTHO: (weiHex) => {
        if (!weiHex || typeof weiHex !== 'string' || !weiHex.startsWith('0x')) {
            return weiHex;
        }
        try {
            const wei = BigInt(weiHex);
            const vtho = Number(wei) / 1e18;
            return vtho.toFixed(6) + ' VTHO';
        } catch (e) {
            return weiHex;
        }
    },

    /**
     * Format timestamp to human-readable date
     */
    formatTimestamp: (timestamp) => {
        if (!timestamp || (typeof timestamp !== 'number' && typeof timestamp !== 'string')) {
            return timestamp;
        }
        try {
            const ts = typeof timestamp === 'string' ? parseInt(timestamp, 10) : timestamp;
            const date = new Date(ts * 1000);
            return date.toLocaleString() + ' (' + timestamp + ')';
        } catch (e) {
            return timestamp;
        }
    },
};

/**
 * Recursively format response object to add human-readable values
 */
function formatResponse(obj, path = '', skipValue = false) {
    if (obj === null || obj === undefined) {
        return obj;
    }

    if (Array.isArray(obj)) {
        return obj.map((item, index) => formatResponse(item, `${path}[${index}]`, skipValue));
    }

    if (typeof obj === 'object') {
        const formatted = {};
        for (const [key, value] of Object.entries(obj)) {
            const currentPath = path ? `${path}.${key}` : key;
            
            // Format based on field name
            if (vetKeys.includes(key) && !(key === 'value' && skipValue)) {
                formatted[key] = value;
                formatted[key + '_VET'] = formatters.weiToVET(value);
                formatted[key + '_WEI'] = formatters.hexToDecimal(value);
            } else if (key === 'energy') {
                formatted[key] = value;
                formatted[key + '_VTHO'] = formatters.weiToVTHO(value);
                formatted[key + '_WEI'] = formatters.hexToDecimal(value);
            } else if (decimalKeys.includes(key)) {
                formatted[key] = value;
                formatted[key + '_decimal'] = formatters.hexToDecimal(value);
            } else if (timestampKeys.includes(key)) {
                formatted[key] = value;
                formatted[key + '_formatted'] = formatters.formatTimestamp(value);
            } else {
                formatted[key] = formatResponse(value, currentPath, skipValue);
            }
        }
        return formatted;
    }

    return obj;
}

/**
 * Format JSON response in the response panel
 */
function formatResponsePanel() {
    // Find all response code blocks - Stoplight Elements uses various selectors
    const selectors = [
        'pre[data-testid="response-body"]',
        'pre code',
        '.response-body pre',
        'pre',
        'code'
    ];

    const isStorage = window.location.hash?.includes('address--storage')
    
    let responseElements = [];
    for (const selector of selectors) {
        const elements = document.querySelectorAll(selector);
        elements.forEach(el => {
            // Check if it looks like JSON
            const text = el.textContent?.trim() || '';
            if (text.startsWith('{') || text.startsWith('[')) {
                // Avoid duplicates
                if (!responseElements.find(e => e.contains(el) || el.contains(e))) {
                    responseElements.push(el);
                }
            }
        });
    }
    
    responseElements.forEach(element => {
        try {
            const text = element.textContent?.trim();
            if (!text || text === '') return;
            
            // Skip if already formatted
            if (element.closest('.formatted-response') || 
                element.parentElement?.querySelector('.formatted-response')) {
                return;
            }
            
            // Try to parse as JSON
            const json = JSON.parse(text);
            
            // Format the response
            const formatted = formatResponse(json, '', isStorage);
            
            // Create formatted JSON string
            const formattedJson = JSON.stringify(formatted, null, 2);
            
            // Find the parent container (usually a response section)
            let container = element.parentElement;
            while (container && !container.classList.contains('response') && 
                   !container.getAttribute('data-testid')?.includes('response')) {
                container = container.parentElement;
            }
            if (!container) {
                container = element.parentElement;
            }
            
            // Check if we already added formatted response
            if (container.querySelector('.formatted-response')) {
                return;
            }
            
            // Create a wrapper to show formatted version
            const wrapper = document.createElement('div');
            wrapper.className = 'formatted-response';
            
            const label = document.createElement('div');
            label.className = 'formatted-label';
            label.innerHTML = '📊 <span>Human-Readable Format</span>';
            
            const formattedPre = document.createElement('pre');
            formattedPre.textContent = formattedJson;
            
            wrapper.appendChild(label);
            wrapper.appendChild(formattedPre);
            
            // Insert after the original response
            if (container) {
                container.appendChild(wrapper);
            } else {
                element.parentElement.appendChild(wrapper);
            }
        } catch (e) {
            // Not JSON or parse error, skip
        }
    });
}

// Mutation observer for WebSocket operations
const mutationObserver = new MutationObserver(() => {
    // Remove WebSocket request/response sections
    if (window.location.hash?.includes("#/paths/subscriptions")) {
        const element = document.querySelector('[data-testid="two-column-right"]');
        if (element) {
            element.remove();
        }
    }
    
    // Format response panels
    formatResponsePanel();
});

mutationObserver.observe(document, {attributes: false, childList: true, characterData: false, subtree: true});

// Also format on initial load
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', formatResponsePanel);
} else {
    formatResponsePanel();
}

// Format responses when they appear after API calls
const responseObserver = new MutationObserver(() => {
    formatResponsePanel();
});

// Wait for body to be available before observing
function setupResponseObserver() {
    if (document.body) {
        responseObserver.observe(document.body, {childList: true, subtree: true});
    } else {
        // If body doesn't exist yet, wait for DOMContentLoaded
        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', setupResponseObserver);
        } else {
            // Fallback: try again after a short delay
            setTimeout(setupResponseObserver, 100);
        }
    }
}

setupResponseObserver();
