const express = require('express');
const axios = require('axios');
const cheerio = require('cheerio');

const app = express();
const PORT = 3000;

async function fetchLaptops() {
    let allLaptops = [];
    
    // O site usa paginação via URL, então é necessário percorrer as páginas para garantir que pegue tudo
    for (let i = 1; i <= 20; i++) {
        const URL = `https://webscraper.io/test-sites/e-commerce/static/computers/laptops?page=${i}`;
        
        try {
            // Adicionado um User-Agent simples para a requisição não ser bloqueada de cara
            const { data } = await axios.get(URL, {
                headers: { 'User-Agent': 'Mozilla/5.0' }
            });
            
            const $ = cheerio.load(data);
            const itemsOnPage = $('.thumbnail');

            // Se a página vier vazia, mata o loop para ganhar tempo
            if (itemsOnPage.length === 0) break; 

            itemsOnPage.each((_, el) => {
                const name = $(el).find('.title').text().trim();
                const desc = $(el).find('.description').text().trim();
                
                // Filtro focado no nome, já que foram observadas inconsistências na descrição do site
                if (name.toLowerCase().includes('lenovo') || name.toLowerCase().includes('thinkpad')) {
                    // Limpo o preço e transformo em número para conseguir ordenar depois
                    const price = parseFloat($(el).find('.price').text().replace('$', ''));
                    
                    // Pega o rating e reviews tratando casos onde o valor pode vir zerado
                    const rating = $(el).find('.ratings p[data-rating]').attr('data-rating') || 0;
                    const reviews = $(el).find('.ratings .pull-right').text().replace(/\D/g, '') || 0;
                    
                    // Monta o link completo concatenando a base do site
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
            // Log de erro básico para saber em qual página a extração falhou
            console.error(`Erro na página ${i}:`, err.message);
        }
    }
    
    // Entrega a lista final já ordenada por preço (requisito do teste)
    return allLaptops.sort((a, b) => a.price - b.price);
}

// Rota principal da API que entrega o JSON para quem consumir
app.get('/laptops', async (req, res) => {
    const results = await fetchLaptops();
    res.json(results);
});

app.listen(PORT, () => {
    console.log(`Servidor rodando em http://localhost:${PORT}/laptops`);
});