const express = require('express');
const axios = require('axios');
const cheerio = require('cheerio');

const app = express();
const PORT = 3000;

async function fetchLaptops() {
    let allLaptops = [];
    // O site tem paginação. Vamos percorrer as páginas para achar todos os Lenovo.
    for (let i = 1; i <= 20; i++) {
        const URL = `https://webscraper.io/test-sites/e-commerce/static/computers/laptops?page=${i}`;
        
        try {
            const { data } = await axios.get(URL, {
                headers: { 'User-Agent': 'Mozilla/5.0' }
            });
            const $ = cheerio.load(data);
            const itemsOnPage = $('.thumbnail');

            if (itemsOnPage.length === 0) break; // Para se chegar em uma página vazia

            itemsOnPage.each((_, el) => {
                const name = $(el).find('.title').text().trim();
                const desc = $(el).find('.description').text().trim();
                
                // Filtro: Lenovo ou ThinkPad (linha da Lenovo)
                if (name.toLowerCase().includes('lenovo') || name.toLowerCase().includes('thinkpad')) {
                    const price = parseFloat($(el).find('.price').text().replace('$', ''));
                    const rating = $(el).find('.ratings p[data-rating]').attr('data-rating') || 0;
                    const reviews = $(el).find('.ratings .pull-right').text().replace(/\D/g, '') || 0;
                    const link = 'https://webscraper.io' + $(el).find('.title').attr('href');

                    allLaptops.push({
                        name,
                        price,
                        description: desc,
                        rating: parseInt(rating),
                        reviews: parseInt(reviews),
                        link
                    });
                }
            });
        } catch (err) {
            console.error(`Erro na página ${i}:`, err.message);
        }
    }
    // Ordena do mais barato para o mais caro [cite: 4]
    return allLaptops.sort((a, b) => a.price - b.price);
}

app.get('/laptops', async (req, res) => {
    const results = await fetchLaptops();
    res.json(results);
});

app.listen(PORT, () => {
    console.log(`Servidor rodando em http://localhost:${PORT}/laptops`);
});