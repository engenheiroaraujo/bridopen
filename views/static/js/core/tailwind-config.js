// Bridopen editorial palette. Functional status colors retain their meaning.
tailwind.config = {
  darkMode: "class",
  theme: {
    extend: {
      fontFamily: {
        sans: ['"Segoe UI"', 'Arial', 'sans-serif'],
        serif: ['Georgia', '"Times New Roman"', 'serif'],
      },
      colors: {
        brand: {50:'#fcf6f3',100:'#f4e9e5',200:'#ead2c9',300:'#e8b6a4',400:'#dd9a81',500:'#bd7157',600:'#a34f39',700:'#873f2e',800:'#743728',900:'#623226',950:'#33221c'},
        slate: {50:'#fafaf9',100:'#f2f2f0',200:'#e7e7e3',300:'#d3d3ce',400:'#a0a09a',500:'#787874',600:'#646460',700:'#484845',800:'#393936',900:'#292928',950:'#232322'},
      },
      borderRadius: {md:'0.375rem',lg:'0.5rem',xl:'0.625rem','2xl':'0.75rem'},
    },
  },
};
