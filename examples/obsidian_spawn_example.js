// Example of how an Obsidian plugin would spawn typer with configuration
// This demonstrates the command-line argument approach

const { spawn } = require('child_process');
const msgpack = require('@msgpack/msgpack');

async function spawnTyperWithConfig() {
    // Example: Obsidian plugin settings
    const pluginSettings = {
        maxSuggestions: 8,      // User wants only 8 suggestions max
        minPrefixLength: 2,     // Start suggesting after 2 characters
        maxPrefixLength: 40,    // Don't allow super long prefixes
        enableFiltering: false  // User wants to see all words, no filtering
    };

    // Convert to command line arguments
    const args = [
        '-d',  // debug mode for this example
        // NOTE: These would be server config flags, not CLI flags
        // For now, demonstrating the concept
    ];

    console.log('Spawning typer with config:', pluginSettings);
    console.log('Command line args:', args);

    // Spawn the typer process
    const typer = spawn('./typer', args, {
        stdio: ['pipe', 'pipe', 'pipe']
    });

    // Example completion request
    const request = {
        p: 'hello',  // prefix
        l: pluginSettings.maxSuggestions
    };

    const requestData = msgpack.encode(request);
    console.log(`Sending request for "${request.p}" with limit ${request.l}`);

    typer.stdin.write(requestData);
    typer.stdin.end();

    // Handle response
    let responseData = Buffer.alloc(0);
    
    typer.stdout.on('data', (chunk) => {
        responseData = Buffer.concat([responseData, chunk]);
    });

    typer.stdout.on('end', () => {
        try {
            const response = msgpack.decode(responseData);
            console.log('Response received:');
            console.log(`- Count: ${response.c}`);
            console.log(`- Time: ${response.t} microseconds`);
            console.log('- Suggestions:');
            response.s.forEach((suggestion, index) => {
                console.log(`  ${index + 1}. ${suggestion.w} (rank: ${suggestion.r})`);
            });
        } catch (error) {
            console.error('Failed to decode response:', error);
        }
    });

    typer.stderr.on('data', (data) => {
        console.log('Debug:', data.toString());
    });

    typer.on('close', (code) => {
        console.log(`Typer process exited with code ${code}`);
    });
}

// Run the example
spawnTyperWithConfig().catch(console.error);

/*
FUTURE: When we implement proper server config flags, the spawn would look like:

const typer = spawn('./typer', [
    '-d',
    '--max-limit', pluginSettings.maxSuggestions.toString(),
    '--min-prefix', pluginSettings.minPrefixLength.toString(),
    '--max-prefix', pluginSettings.maxPrefixLength.toString(),
    '--enable-filter=' + pluginSettings.enableFiltering.toString()
]);

This way the Obsidian plugin can:
1. Read user settings from plugin configuration
2. Convert to command line arguments
3. Spawn typer with those arguments
4. Use fast MessagePack communication
5. No need for environment variables or config files!
*/
